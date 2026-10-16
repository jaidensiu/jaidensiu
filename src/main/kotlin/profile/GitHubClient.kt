package profile

import java.io.IOException
import java.net.URI
import java.net.http.HttpClient
import java.net.http.HttpRequest
import java.net.http.HttpResponse
import kotlin.random.Random
import kotlin.time.Duration
import kotlin.time.Duration.Companion.seconds
import kotlin.time.toJavaDuration
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.put

class GitHubException(
    message: String,
    val status: Int? = null,
    val attempts: Int = 1,
    cause: Throwable? = null,
) : RuntimeException(message, cause)

data class HttpResult(val status: Int, val body: String, val retryAfter: Duration? = null)

fun interface Transport {
    /** POSTs [body] to the GraphQL endpoint. Throws [IOException] on network failure or timeout. */
    fun post(body: String): HttpResult
}

class HttpTransport(private val token: String, private val requestTimeout: Duration = 30.seconds) : Transport {
    private val client = HttpClient.newBuilder().connectTimeout(10.seconds.toJavaDuration()).build()

    override fun post(body: String): HttpResult {
        val request = HttpRequest.newBuilder(URI.create("https://api.github.com/graphql"))
            .timeout(requestTimeout.toJavaDuration())
            .header("Authorization", "Bearer $token")
            .header("Accept", "application/vnd.github+json")
            .header("User-Agent", "profile-card")
            .POST(HttpRequest.BodyPublishers.ofString(body))
            .build()
        val response = client.send(request, HttpResponse.BodyHandlers.ofString())
        val headers = response.headers()
        val retryAfter = headers.firstValue("retry-after").map { it.toLongOrNull()?.seconds }.orElse(null)
        // primary rate limit exhausted: GitHub reports it as 403/429 with remaining=0 and a reset epoch
            ?: if (headers.firstValue("x-ratelimit-remaining").orElse(null) == "0") {
                headers.firstValue("x-ratelimit-reset").orElse(null)?.toLongOrNull()
                    ?.let { (it - System.currentTimeMillis() / 1000).coerceAtLeast(1).seconds }
            } else {
                null
            }
        return HttpResult(response.statusCode(), response.body(), retryAfter)
    }
}

/**
 * Read-only GraphQL client. Every query is idempotent, so transient failures
 * (network errors, timeouts, 5xx, rate limiting) are retried with capped
 * exponential backoff and full jitter. Anything else fails immediately.
 */
class GitHubClient(
    private val transport: Transport,
    private val maxAttempts: Int = 4,
    private val baseDelay: Duration = 1.seconds,
    private val maxDelay: Duration = 60.seconds,
    private val sleep: (Duration) -> Unit = { Thread.sleep(it.inWholeMilliseconds) },
    private val random: Random = Random.Default,
) {
    var retries = 0
        private set

    fun graphql(query: String, variables: Map<String, String?> = emptyMap()): JsonObject {
        val body = buildJsonObject {
            put("query", query)
            put("variables", buildJsonObject { variables.forEach { (k, v) -> put(k, v) } })
        }.toString()
        val response = send(body)
        val json = try {
            Json.parseToJsonElement(response.body).jsonObject
        } catch (e: IllegalArgumentException) {
            throw GitHubException("GitHub GraphQL returned a non-JSON body", response.status, cause = e)
        }
        json["errors"]?.takeUnless { it is JsonNull }?.let {
            throw GitHubException("GitHub GraphQL errors: $it", response.status)
        }
        return json["data"]?.takeUnless { it is JsonNull }?.jsonObject
            ?: throw GitHubException("GitHub GraphQL response has no data", response.status)
    }

    private fun send(body: String): HttpResult {
        for (attempt in 1..maxAttempts) {
            val result = try {
                transport.post(body)
            } catch (e: IOException) {
                awaitRetry(attempt, "${e::class.simpleName}: ${e.message}", status = null, retryAfter = null, cause = e)
                continue
            }
            if (result.status in 200..299) return result
            val failure = "HTTP ${result.status}: ${result.body.take(200).replace(Regex("\\s+"), " ")}"
            val retryable = result.status >= 500 || result.status == 429 ||
                    (result.status == 403 && result.retryAfter != null)
            if (!retryable) throw GitHubException(
                "GitHub GraphQL failed, not retrying: $failure",
                result.status,
                attempt
            )
            awaitRetry(attempt, failure, result.status, result.retryAfter, cause = null)
        }
        error("unreachable: awaitRetry throws on the last attempt")
    }

    /** Sleeps before the next attempt, or throws if this was the last one or the server wants too long a pause. */
    private fun awaitRetry(attempt: Int, failure: String, status: Int?, retryAfter: Duration?, cause: Throwable?) {
        if (attempt >= maxAttempts) {
            throw GitHubException("GitHub GraphQL failed after $attempt attempts: $failure", status, attempt, cause)
        }
        if (retryAfter != null && retryAfter > maxDelay) {
            throw GitHubException(
                "GitHub asked to wait $retryAfter (> $maxDelay), giving up: $failure",
                status,
                attempt,
                cause
            )
        }
        val delay = retryAfter ?: backoff(attempt)
        retries++
        System.err.println("warn: github graphql retry attempt=$attempt/$maxAttempts delay=$delay status=$status cause=$failure")
        sleep(delay)
    }

    private fun backoff(attempt: Int): Duration {
        val cap = minOf(maxDelay, baseDelay * (1 shl (attempt - 1)).toDouble())
        return cap * random.nextDouble()
    }
}

internal fun JsonElement.obj(key: String): JsonObject = jsonObject.getValue(key).jsonObject

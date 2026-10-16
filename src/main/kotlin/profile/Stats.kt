package profile

import java.time.LocalDate
import java.time.OffsetDateTime
import java.time.ZoneOffset
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.boolean
import kotlinx.serialization.json.int
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

data class Stats(
    val repos: Int,
    val contributed: Int,
    val stars: Int,
    val commits: Int,
    val followers: Int,
    val locAdded: Long,
    val locDeleted: Long,
    val joined: LocalDate,
) {
    val loc get() = locAdded - locDeleted
}

private val USER_QUERY = $$"""
    query($login: String!, $cursor: String) {
      user(login: $login) {
        id
        createdAt
        followers { totalCount }
        repositoriesContributedTo(first: 1, contributionTypes: [COMMIT, PULL_REQUEST, REPOSITORY]) { totalCount }
        repositories(first: 100, after: $cursor, ownerAffiliations: OWNER) {
          totalCount
          pageInfo { hasNextPage endCursor }
          nodes { name stargazerCount isFork }
        }
      }
    }
""".trimIndent()

private val LOC_QUERY = $$"""
    query($owner: String!, $name: String!, $id: ID!, $cursor: String) {
      repository(owner: $owner, name: $name) {
        defaultBranchRef { target { ... on Commit {
          history(first: 100, author: { id: $id }, after: $cursor) {
            pageInfo { hasNextPage endCursor }
            nodes { additions deletions }
          }
        } } }
      }
    }
""".trimIndent()

private fun JsonObject.count(key: String) = obj(key).getValue("totalCount").jsonPrimitive.int

fun fetchStats(gh: GitHubClient, login: String): Stats {
    var user: JsonObject
    val repos = mutableListOf<JsonObject>()
    var cursor: String? = null
    do {
        user = gh.graphql(USER_QUERY, mapOf("login" to login, "cursor" to cursor)).obj("user")
        val page = user.obj("repositories")
        repos += page.getValue("nodes").jsonArray.map { it.jsonObject }
        val info = page.obj("pageInfo")
        cursor = info.getValue("endCursor").takeUnless { it is JsonNull }?.jsonPrimitive?.content
    } while (info.getValue("hasNextPage").jsonPrimitive.boolean)

    val userId = user.getValue("id").jsonPrimitive.content
    val joined = OffsetDateTime.parse(user.getValue("createdAt").jsonPrimitive.content).toLocalDate()
    val (added, deleted) = repos
        .filterNot { it.getValue("isFork").jsonPrimitive.boolean }
        .map { linesOfCode(gh, login, it.getValue("name").jsonPrimitive.content, userId) }
        .fold(0L to 0L) { (a, d), (ra, rd) -> (a + ra) to (d + rd) }

    return Stats(
        repos = user.obj("repositories").getValue("totalCount").jsonPrimitive.int,
        contributed = user.count("repositoriesContributedTo"),
        stars = repos.sumOf { it.getValue("stargazerCount").jsonPrimitive.int },
        commits = commits(gh, login, joined.year),
        followers = user.count("followers"),
        locAdded = added,
        locDeleted = deleted,
        joined = joined,
    )
}

/** Commit contributions summed per year since joining. Private commits count only if the token can see them. */
private fun commits(gh: GitHubClient, login: String, joinedYear: Int): Int {
    val years = (joinedYear..OffsetDateTime.now(ZoneOffset.UTC).year).joinToString("\n") { y ->
        """y$y: contributionsCollection(from: "$y-01-01T00:00:00Z", to: "${y + 1}-01-01T00:00:00Z") """ +
                "{ totalCommitContributions }"
    }
    val query = "query(\$login: String!) { user(login: \$login) { $years } }"
    return gh.graphql(query, mapOf("login" to login)).obj("user").values.sumOf {
        it.jsonObject.getValue("totalCommitContributions").jsonPrimitive.int
    }
}

/** Additions/deletions of the user's own commits on a repo's default branch. */
private fun linesOfCode(gh: GitHubClient, owner: String, repo: String, userId: String): Pair<Long, Long> {
    var added = 0L
    var deleted = 0L
    var cursor: String? = null
    while (true) {
        val vars = mapOf("owner" to owner, "name" to repo, "id" to userId, "cursor" to cursor)
        val ref = gh.graphql(LOC_QUERY, vars).obj("repository")["defaultBranchRef"]
        if (ref == null || ref is JsonNull) return added to deleted // empty repo
        val history = ref.obj("target").obj("history")
        for (node in history.getValue("nodes").jsonArray) {
            added += node.jsonObject.getValue("additions").jsonPrimitive.int
            deleted += node.jsonObject.getValue("deletions").jsonPrimitive.int
        }
        val info = history.obj("pageInfo")
        if (!info.getValue("hasNextPage").jsonPrimitive.boolean) return added to deleted
        cursor = info.getValue("endCursor").jsonPrimitive.content
    }
}

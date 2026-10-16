package profile

import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.StandardCopyOption
import java.time.LocalDate
import java.time.ZoneOffset
import kotlin.system.exitProcess

/**
 * Regenerates dark_mode.svg / light_mode.svg with live GitHub stats.
 *
 * Usage: profile-card [output-dir]. Reads ACCESS_TOKEN (a PAT, sees private repos)
 * or falls back to GITHUB_TOKEN. Any failure exits non-zero without touching the
 * existing SVGs, so the README keeps showing the last good card.
 */
fun main(args: Array<String>) {
    val outDir = Path.of(args.firstOrNull() ?: ".")
    val token = sequenceOf("ACCESS_TOKEN", "GITHUB_TOKEN").firstNotNullOfOrNull {
        System.getenv(it)?.takeIf(String::isNotBlank)
    } ?: fail("error: set ACCESS_TOKEN or GITHUB_TOKEN")
    val gh = GitHubClient(HttpTransport(token))
    val stats = try {
        fetchStats(gh, Profile.LOGIN)
    } catch (e: GitHubException) {
        fail("error: dependency=github-graphql status=${e.status} attempts=${e.attempts} retries=${gh.retries} cause=${e.message}")
    } catch (e: RuntimeException) {
        // unexpected response shape (missing field, wrong type): treat as a hard failure, never render partial stats
        fail("error: dependency=github-graphql failure=unexpected-response retries=${gh.retries} cause=$e")
    }

    val today = LocalDate.now(ZoneOffset.UTC)
    // render both before writing either, so a layout error can't leave the two themes out of sync
    val cards = Palette.entries.associateWith { render(it, stats, today) }
    for ((palette, svg) in cards) {
        val target = outDir.resolve("${palette.name.lowercase()}_mode.svg")
        val tmp = Files.createTempFile(outDir, target.fileName.toString(), ".tmp")
        Files.writeString(tmp, svg)
        Files.move(tmp, target, StandardCopyOption.REPLACE_EXISTING, StandardCopyOption.ATOMIC_MOVE)
    }
    if (gh.retries > 0) System.err.println("warn: rendered after ${gh.retries} github retries")
}

private fun fail(message: String): Nothing {
    System.err.println(message)
    exitProcess(1)
}

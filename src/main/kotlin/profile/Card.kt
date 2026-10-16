package profile

import java.text.NumberFormat
import java.time.LocalDate
import java.time.Period
import java.util.Locale

/** Width of the info column in characters; every line is padded with dots to exactly this. */
const val WIDTH = 56

private val ART = """
           ==.             -=.
           -*+:    ..    .=+=
            -+=-----------+-
           :-==-------------:.
         :=================--=:
        -==-+@%==========#@*----
       :+=--+#+++++++++==+*+---=-
       ++====+++++++++++++=======.
       +*#####%%%%###########***=:
   .---*+++++++*+++++++++++++=====--:
  .=+*%*++++++***++++++++++++====##++:
 .=**##*++++++***++++++++++++====*%*+=.
 -+*#%=*++++++***++++++++++++====-%#*+-
.+**#%.*++++++***+++++++++++++==+-##*+=.
:+**## *++++******++++++++++++==+:*#**+:
-***## +****************+++++++++.+#**+:
:*##%+ -*******************++++++ =%#**:
 :==-   ************************:  -==:
        .###############******#-
          =%%%%%%%%%########%+.
           -%%@@@@@@@%%%%%%#+
           -**###%@=-%%%##**+
           -**####%: %%##***+
           -****##%- %###***+
           :+******:.+*##**+-
""".trim('\n').lines()

// IntelliJ "Islands" themes: a frame with rounded panels, text in the default editor color scheme
// (header = keyword, key = property, added = string, dim = line numbers).
enum class Palette(
    val frame: String, val island: String, val border: String, val art: String, val header: String,
    val key: String, val value: String, val dim: String, val added: String, val deleted: String,
) {
    DARK("#131314", "#1e1f22", "#2b2d30", "#3ddc84", "#cf8e6d", "#c77dbb", "#bcbec4", "#4b5059", "#6aab73", "#f75464"),
    LIGHT("#f2f3f5", "#ffffff", "#dfe1e5", "#1f9d55", "#0033b3", "#871094", "#080808", "#aeb3c2", "#067d17", "#c7222d"),
}

private enum class Tone { HEADER, KEY, VALUE, DIM, ADDED, DELETED }

private data class Span(val text: String, val tone: Tone)

private typealias Line = List<Span>

private fun kv(key: String, value: Any, width: Int = WIDTH): Line {
    val dots = width - key.length - value.toString().length - 3
    require(dots >= 1) { "\"$key: $value\" does not fit in $width columns" }
    return listOf(Span("$key: ", Tone.KEY), Span(".".repeat(dots) + " ", Tone.DIM), Span(value.toString(), Tone.VALUE))
}

private fun kv2(k1: String, v1: Any, k2: String, v2: Any): Line =
    kv(k1, v1, 32) + Span(" | ", Tone.DIM) + kv(k2, v2, WIDTH - 35)

private fun rule(title: String): Line {
    val label = "─ $title "
    return listOf(Span(label, Tone.HEADER), Span("─".repeat(WIDTH - label.length), Tone.DIM))
}

fun uptime(since: LocalDate, today: LocalDate): String {
    val p = Period.between(since, today)
    fun unit(n: Int, name: String) = "$n $name" + if (n == 1) "" else "s"
    return "${unit(p.years, "year")}, ${unit(p.months, "month")}, ${unit(p.days, "day")}"
}

private fun infoLines(s: Stats, today: LocalDate): List<Line> {
    val n = { x: Number -> NumberFormat.getIntegerInstance(Locale.US).format(x) }
    val title = "${Profile.LOGIN} "
    return listOf(
        listOf(Span(title, Tone.HEADER), Span("─".repeat(WIDTH - title.length), Tone.DIM)),
        emptyList(),
        kv("Uptime", uptime(s.joined, today)),
        kv("Interests", Profile.INTERESTS),
        kv("Hobbies", Profile.HOBBIES),
        emptyList(),
        rule("Contact"),
        kv("Website", Profile.WEBSITE),
        kv("Blog", Profile.BLOG),
        kv("LinkedIn", Profile.LINKEDIN),
        emptyList(),
        rule("GitHub Stats"),
        kv2("Repos", "${s.repos} {Contributed: ${s.contributed}}", "Stars", n(s.stars)),
        kv2("Commits", n(s.commits), "Followers", n(s.followers)),
        listOf(
            Span("Lines of Code: ", Tone.KEY), Span(n(s.loc), Tone.VALUE), Span(" ( ", Tone.DIM),
            Span(n(s.locAdded) + "++", Tone.ADDED), Span(", ", Tone.DIM),
            Span(n(s.locDeleted) + "--", Tone.DELETED), Span(" )", Tone.DIM),
        ),
    )
}

private fun String.xml() =
    replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;").replace("\"", "&quot;")

fun render(palette: Palette, stats: Stats, today: LocalDate): String {
    fun color(tone: Tone) = when (tone) {
        Tone.HEADER -> palette.header
        Tone.KEY -> palette.key
        Tone.VALUE -> palette.value
        Tone.DIM -> palette.dim
        Tone.ADDED -> palette.added
        Tone.DELETED -> palette.deleted
    }
    return buildString {
        appendLine(
            """<svg xmlns="http://www.w3.org/2000/svg" width="840" height="500" viewBox="0 0 840 500" """ +
                    """font-family="JetBrains Mono, Consolas, Menlo, monospace" font-size="13px">""",
        )
        appendLine("""<rect width="840" height="500" rx="14" fill="${palette.frame}"/>""")
        for ((x, w) in listOf(10 to 345, 365 to 465)) {
            appendLine("""<rect x="${x + 0.5}" y="10.5" width="$w" height="479" rx="10" fill="${palette.island}" stroke="${palette.border}"/>""")
        }
        ART.forEachIndexed { i, row ->
            appendLine("""<text x="22" y="${255 - (ART.size - 1) * 15 / 2 + i * 15}" fill="${palette.art}" xml:space="preserve">${row.xml()}</text>""")
        }
        infoLines(stats, today).forEachIndexed { i, line ->
            if (line.isEmpty()) return@forEachIndexed
            val spans = line.joinToString("") { """<tspan fill="${color(it.tone)}">${it.text.xml()}</tspan>""" }
            appendLine("""<text x="380" y="${70 + i * 24}" xml:space="preserve">$spans</text>""")
        }
        append("</svg>\n")
    }
}

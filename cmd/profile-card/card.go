package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// width of the info column in characters; every line is padded with dots to exactly this.
const width = 56

var art = strings.Split(strings.Trim(`
       :=:                     .=-
       .+*:                   :+*:
         =*-  :--=======--:. :*+.
         .=#*###############*#+:
       -+#######################+-.
    .-*###########################*=:
   :*#####*+*###############*++#####*-
  :#######: :###############- :*######-
 :*########*#################*#########-
 +#####################################*
 +++++++++++++++++++++++++++++++++++++++:
`, "\n"), "\n")

// IntelliJ "Islands" themes: a frame with rounded panels, text in the default editor color scheme
// (header = keyword, key = property, added = string, dim = line numbers).
type palette struct {
	name, frame, island, border, art, header, key, value, dim, added, deleted string
}

var palettes = []palette{
	{"dark", "#131314", "#1e1f22", "#2b2d30", "#3ddc84", "#cf8e6d", "#c77dbb", "#bcbec4", "#4b5059", "#6aab73", "#f75464"},
	{"light", "#f2f3f5", "#ffffff", "#dfe1e5", "#1f9d55", "#0033b3", "#871094", "#080808", "#aeb3c2", "#067d17", "#c7222d"},
}

type tone int

const (
	toneHeader tone = iota
	toneKey
	toneValue
	toneDim
	toneAdded
	toneDeleted
)

func (p palette) color(t tone) string {
	return [...]string{p.header, p.key, p.value, p.dim, p.added, p.deleted}[t]
}

type span struct {
	text string
	tone tone
}

type line []span

func runes(s string) int { return utf8.RuneCountInString(s) }

func kv(key, value string, w int) line {
	dots := w - runes(key) - runes(value) - 3
	if dots < 1 {
		panic(fmt.Sprintf("%q does not fit in %d columns", key+": "+value, w))
	}
	return line{{key + ": ", toneKey}, {strings.Repeat(".", dots) + " ", toneDim}, {value, toneValue}}
}

func kv2(k1, v1, k2, v2 string) line {
	return append(append(kv(k1, v1, 32), span{" | ", toneDim}), kv(k2, v2, width-35)...)
}

func rule(label string) line {
	return line{{label, toneHeader}, {strings.Repeat("─", width-runes(label)), toneDim}}
}

// uptime matches java.time.Period.between: a negative day count borrows from the month,
// counting days from since+months (clamped to month end).
func uptime(since, today time.Time) string {
	months := (today.Year()-since.Year())*12 + int(today.Month()-since.Month())
	days := today.Day() - since.Day()
	if days < 0 {
		months--
		days = int(date(today).Sub(addMonthsClamped(since, months)).Hours() / 24)
	}
	unit := func(n int, name string) string {
		if n == 1 {
			return "1 " + name
		}
		return strconv.Itoa(n) + " " + name + "s"
	}
	return unit(months/12, "year") + ", " + unit(months%12, "month") + ", " + unit(days, "day")
}

func date(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func daysIn(y int, m time.Month) int { return time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day() }

func addMonthsClamped(t time.Time, months int) time.Time {
	first := time.Date(t.Year(), t.Month()+time.Month(months), 1, 0, 0, 0, 0, time.UTC)
	return first.AddDate(0, 0, min(t.Day(), daysIn(first.Year(), first.Month()))-1)
}

// commas formats n with US thousands separators.
func commas[T int | int64](n T) string {
	s := strconv.FormatInt(int64(n), 10)
	sign := ""
	if n < 0 {
		sign, s = "-", s[1:]
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return sign + s
}

func infoLines(s Stats, today time.Time) []line {
	return []line{
		rule(login + " "),
		nil,
		kv("Uptime", uptime(s.Joined, today), width),
		kv("Interests", interests, width),
		kv("Hobbies", hobbies, width),
		nil,
		rule("─ Contact "),
		kv("Website", website, width),
		kv("Blog", blog, width),
		kv("LinkedIn", linkedIn, width),
		nil,
		rule("─ GitHub Stats "),
		kv2("Repos", fmt.Sprintf("%d {Contributed: %d}", s.Repos, s.Contributed), "Stars", commas(s.Stars)),
		kv2("Commits", commas(s.Commits), "Followers", commas(s.Followers)),
		{
			{"Lines of Code: ", toneKey}, {commas(s.Loc()), toneValue}, {" ( ", toneDim},
			{commas(s.LocAdded) + "++", toneAdded}, {", ", toneDim},
			{commas(s.LocDeleted) + "--", toneDeleted}, {" )", toneDim},
		},
	}
}

var xmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")

func render(p palette, s Stats, today time.Time) string {
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="840" height="500" viewBox="0 0 840 500" ` +
		`font-family="JetBrains Mono, Consolas, Menlo, monospace" font-size="13px">` + "\n")
	fmt.Fprintf(&b, `<rect width="840" height="500" rx="14" fill="%s"/>`+"\n", p.frame)
	for _, r := range [][2]int{{10, 345}, {365, 465}} {
		fmt.Fprintf(&b, `<rect x="%v" y="10.5" width="%d" height="479" rx="10" fill="%s" stroke="%s"/>`+"\n", float64(r[0])+0.5, r[1], p.island, p.border)
	}
	for i, row := range art {
		fmt.Fprintf(&b, `<text x="22" y="%d" fill="%s" xml:space="preserve">%s</text>`+"\n", 255-(len(art)-1)*15/2+i*15, p.art, xmlEscaper.Replace(row))
	}
	for i, l := range infoLines(s, today) {
		if len(l) == 0 {
			continue
		}
		fmt.Fprintf(&b, `<text x="380" y="%d" xml:space="preserve">`, 70+i*24)
		for _, sp := range l {
			fmt.Fprintf(&b, `<tspan fill="%s">%s</tspan>`, p.color(sp.tone), xmlEscaper.Replace(sp.text))
		}
		b.WriteString("</text>\n")
	}
	b.WriteString("</svg>\n")
	return b.String()
}

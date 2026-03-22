package i18n

// Verses — rotating classical lines for UI decoration.
// Used in wizard pages, loading states, idle indicators, etc.
// Each entry is a pair: [0] = Chinese, [1] = English.
//
// From 菩提偈 (Bodhi Verses), Platform Sutra — three stanzas by 慧能.

var Verses = [][2]string{
	// 其一
	{"菩提本无树", "Bodhi is not a tree"},
	{"明镜亦非台", "Nor the mirror a stand"},
	{"佛性常清净", "Buddha-nature is ever pure"},
	{"何处有尘埃", "Where could dust arise?"},
	// 其二
	{"身是菩提树", "The body is a Bodhi tree"},
	{"心如明镜台", "The mind a mirror bright"},
	{"明镜本清净", "The mirror is pristine by nature"},
	{"何处染尘埃", "How could dust defile it?"},
	// 其三
	{"菩提本无树", "Bodhi is not a tree"},
	{"明镜亦非台", "Nor the mirror a stand"},
	{"本来无一物", "Nothing has ever existed"},
	{"何处惹尘埃", "Where could dust alight?"},
}

// Verse returns the verse for the given index (wraps around).
// Returns Chinese if lang != "en", English otherwise.
func Verse(index int, lang string) string {
	v := Verses[index%len(Verses)]
	if lang == "en" {
		return v[1]
	}
	return v[0]
}

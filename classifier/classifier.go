// Package classifier tague chaque channel (kick, snare, 808, hihat...) a
// partir de son nom, avec repli sur le nom de fichier du sample.
package classifier

import (
	"regexp"
	"strings"

	"flpmine/flp"
)

type rule struct {
	label    string
	patterns []*regexp.Regexp
}

// L'ordre est important : les motifs les plus specifiques doivent etre
// testes avant les plus generiques.
var rules = []rule{
	{"kick", compileAll(`\bkick\b`, `\bkck\b`, `\bbd\b`)},
	{"808", compileAll(`\b808\b`)},
	{"snare", compileAll(`\bsnare\b`, `\bsn\b`)},
	{"clap", compileAll(`\bclap\b`)},
	{"rim", compileAll(`\brim(shot)?\b`)},
	{"hihat", compileAll(`\bhi[\s\-_]?hat\b`, `\bhh\b`, `\bhat\b`)},
	{"open_hat", compileAll(`\bopen[\s\-_]?hat\b`, `\boh\b`)},
	{"crash", compileAll(`\bcrash\b`)},
	{"cymbal", compileAll(`\bcymbal\b`, `\bride\b`)},
	{"shaker", compileAll(`\bshaker\b`)},
	{"perc", compileAll(`\bperc(ussion)?\b`)},
	{"fx", compileAll(`\bfx\b`, `\bsfx\b`, `\briser?\b`, `\bdrop\b`, `\bswell\b`, `\btransition\b`)},
	{"fill", compileAll(`\bfill\b`, `\broll\b`)},
	{"bass", compileAll(`\bbass\b`)},
	{"melody", compileAll(`\bmelo(dy)?\b`, `\bpiano\b`, `\bkeys?\b`, `\bchords?\b`, `\bguitar\b`, `\bstrings?\b`, `\brhodes\b`)},
	{"vocal", compileAll(`\bvocal\b`, `\btags?\b`, `\bvox\b`, `\backapella\b`, `\bad[\s\-_]?lib\b`)},
}

func compileAll(patterns ...string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, len(patterns))
	for i, p := range patterns {
		out[i] = regexp.MustCompile(`(?i)` + p)
	}
	return out
}

// normalize remplace les separateurs (_, -, .) par des espaces pour que les
// motifs \b (frontiere de mot) fonctionnent correctement — sans ca,
// "\bkick\b" ne matche pas "TSTN2_kick_oneshot" car "_" est considere comme
// un caractere de mot par le moteur regex.
func normalize(s string) string {
	repl := strings.NewReplacer("_", " ", "-", " ", ".", " ")
	return repl.Replace(s)
}

// ClassifyText retourne le premier label dont un motif matche dans le texte.
func ClassifyText(text string) string {
	if text == "" {
		return "unknown"
	}
	n := normalize(text)
	for _, r := range rules {
		for _, pat := range r.patterns {
			if pat.MatchString(n) {
				return r.label
			}
		}
	}
	return "unknown"
}

// ClassifyChannel classifie un channel a partir de son nom, avec repli sur
// le nom de fichier du sample. Le nom du channel est prioritaire (choix
// explicite de l'utilisateur dans FL Studio) ; le nom de fichier n'est
// utilise que si le nom du channel n'a rien donne.
func ClassifyChannel(ch flp.Channel) string {
	label := ClassifyText(ch.Name)
	if label == "unknown" {
		label = ClassifyText(ch.SampleFilename)
	}
	if label == "unknown" && ch.Kind.String() == "Instrument" {
		return "melody"
	}
	return label
}

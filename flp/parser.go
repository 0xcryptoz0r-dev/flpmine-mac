// Package flp implemente un parser minimal du format de projet FL Studio
// (.flp), suffisant pour extraire les channels (instruments/samplers), les
// patterns et leurs notes MIDI. Aucune dependance externe : tout est fait
// avec la bibliotheque standard Go.
//
// Le format .flp n'est pas documente officiellement par Image-Line, mais sa
// structure binaire (header FLhd/FLdt + flux d'evenements ID+valeur) est
// connue de la communaute (voir le projet pyflp, entre autres). Les
// constantes d'ID ci-dessous sont extraites de cette connaissance publique.
package flp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"strings"
)

// --- Constantes de base du format (tailles d'evenement selon leur ID) ---

const (
	sizeByte  = 0   // IDs 0-63   : valeur sur 1 octet
	sizeWord  = 64  // IDs 64-127 : valeur sur 2 octets
	sizeDword = 128 // IDs 128-191: valeur sur 4 octets
	sizeText  = 192 // IDs 192-207: texte, longueur prefixee (VarInt)
	sizeData  = 208 // IDs >=208  : donnees brutes, longueur prefixee (VarInt)
)

// --- IDs d'evenements utilises (voir commentaire de package) ---

const (
	idChannelNew        = sizeWord            // 64
	idChannelType       = 21                  // BYTE
	idChannelNameOld    = sizeText            // 192 (TEXT + 0) — deprecated, encore utilise en repli
	idChannelName       = sizeText + 11       // 203 (PluginID.Name) — nom reellement affiche
	idChannelSamplePath = sizeText + 4        // 196
	idPatternNew        = sizeWord + 1        // 65
	idPatternName       = sizeText + 1        // 193
	idPatternNotes      = sizeData + 16       // 224
	idProjectTempo      = sizeDword + 28      // 156
	idProjectFLVersion  = sizeText + 7        // 199 (toujours ASCII)
)

// ChannelKind est le type d'un channel (Sampler, Instrument, etc.)
type ChannelKind int

const (
	KindSampler ChannelKind = 0
	KindNative  ChannelKind = 2
	KindLayer   ChannelKind = 3
	KindInstrument ChannelKind = 4
	KindAutomation ChannelKind = 5
)

func (k ChannelKind) String() string {
	// Regroupement aligne sur les categories utiles pour le classement
	// (Native et Instrument sont tous deux des plugins generateurs, sans
	// distinction utile pour nous ; on les regroupe sous "Instrument").
	switch k {
	case KindSampler:
		return "Sampler"
	case KindLayer:
		return "Layer"
	case KindAutomation:
		return "Automation"
	case KindNative, KindInstrument:
		return "Instrument"
	default:
		return "Unknown"
	}
}

// Channel represente un channel du rack (instrument ou sampler).
type Channel struct {
	Index          int
	Kind           ChannelKind
	Name           string
	SamplePath     string
	SampleFilename string
}

// Note represente une note MIDI dans un pattern (24 octets dans le fichier).
type Note struct {
	Position     uint32
	RackChannel  uint16
	Length       uint32
	Key          uint16
	Velocity     uint8
	Pan          uint8
}

// Pattern represente un pattern (groove) avec ses notes.
type Pattern struct {
	Index int
	Name  string
	Notes []Note
}

// Project est le resultat de l'extraction d'un fichier .flp.
type Project struct {
	SourcePath string
	PPQ        uint16
	TempoBPM   float64 // 0 si absent
	Channels   []Channel
	Patterns   []Pattern
}

// ChannelIndexes retourne l'ensemble (trie) des index de channels utilises
// dans les notes de ce pattern.
func (p Pattern) ChannelIndexes() []int {
	seen := map[int]bool{}
	var out []int
	for _, n := range p.Notes {
		idx := int(n.RackChannel)
		if !seen[idx] {
			seen[idx] = true
			out = append(out, idx)
		}
	}
	// tri simple par insertion (peu d'elements)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// Parse lit et parse un fichier .flp.
func Parse(path string) (*Project, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("lecture du fichier: %w", err)
	}
	return ParseBytes(path, data)
}

// ParseBytes parse un fichier .flp deja charge en memoire.
func ParseBytes(sourcePath string, data []byte) (*Project, error) {
	if len(data) < 22 {
		return nil, errors.New("fichier trop court pour etre un .flp valide")
	}
	if string(data[0:4]) != "FLhd" {
		return nil, errors.New("magic FLhd manquant : ce n'est pas un fichier .flp")
	}
	headerLen := binary.LittleEndian.Uint32(data[4:8])
	if headerLen != 6 {
		return nil, fmt.Errorf("taille de header inattendue: %d", headerLen)
	}
	ppq := binary.LittleEndian.Uint16(data[12:14])

	if string(data[14:18]) != "FLdt" {
		return nil, errors.New("magic FLdt manquant : chunk de donnees introuvable")
	}
	eventsSize := binary.LittleEndian.Uint32(data[18:22])
	if int(22+eventsSize) > len(data) {
		return nil, errors.New("taille du chunk de donnees corrompue (fichier tronque ?)")
	}

	proj := &Project{SourcePath: sourcePath, PPQ: ppq}

	pos := 22
	end := 22 + int(eventsSize)

	useUnicode := true // par defaut (FL Studio >= 11.5, cas de la grande majorite des projets actuels)
	currentChannel := -1
	currentPattern := -1
	patternByIdx := map[int]int{} // index de pattern (tel que stocke dans le fichier) -> index dans proj.Patterns
	// Un channel peut heberger un plugin d'effet interne (ex: un delay) qui
	// emet lui aussi un evenement PluginID.Name — on ne garde que la toute
	// premiere occurrence par channel (celle du generateur/sampler
	// lui-meme), pour ne pas se faire ecraser le nom par un effet charge
	// dessus.
	channelNameLocked := map[int]bool{}

	for pos < end {
		id := int(data[pos])
		pos++

		var value []byte
		switch {
		case id < sizeWord:
			value = data[pos : pos+1]
			pos += 1
		case id < sizeDword:
			value = data[pos : pos+2]
			pos += 2
		case id < sizeText:
			value = data[pos : pos+4]
			pos += 4
		default:
			n, consumed, err := readVarInt(data[pos:])
			if err != nil {
				return nil, fmt.Errorf("varint invalide a l'offset %d: %w", pos, err)
			}
			pos += consumed
			if pos+n > len(data) {
				return nil, fmt.Errorf("longueur d'evenement depasse la fin du fichier (id=%d)", id)
			}
			value = data[pos : pos+n]
			pos += n
		}

		switch id {
		case idProjectFLVersion:
			// Determine l'encodage du texte pour la suite du fichier.
			// (Les versions FL Studio < 11.5 utilisent de l'ASCII simple.)
			verStr := strings.TrimRight(string(value), "\x00")
			useUnicode = flVersionIsUnicode(verStr)

		case idChannelNew:
			currentChannel++
			proj.Channels = append(proj.Channels, Channel{Index: currentChannel})

		case idChannelType:
			if currentChannel >= 0 && currentChannel < len(proj.Channels) {
				proj.Channels[currentChannel].Kind = ChannelKind(value[0])
			}

		case idChannelName:
			if currentChannel >= 0 && currentChannel < len(proj.Channels) && !channelNameLocked[currentChannel] {
				proj.Channels[currentChannel].Name = decodeText(value, useUnicode)
				channelNameLocked[currentChannel] = true
			}

		case idChannelNameOld:
			// ID deprecated : ne l'utiliser que si aucun nom "moderne" n'a
			// deja ete verrouille pour ce channel.
			if currentChannel >= 0 && currentChannel < len(proj.Channels) && !channelNameLocked[currentChannel] {
				proj.Channels[currentChannel].Name = decodeText(value, useUnicode)
			}

		case idChannelSamplePath:
			if currentChannel >= 0 && currentChannel < len(proj.Channels) {
				sp := decodeText(value, useUnicode)
				proj.Channels[currentChannel].SamplePath = sp
				proj.Channels[currentChannel].SampleFilename = filenameFromWindowsPath(sp)
				// FL Studio garde certains channels types "Instrument" tant
				// qu'aucun fichier audio n'est charge dedans (ex: un clip
				// audio glisse dans le channel rack) ; des qu'un sample est
				// present, on le traite comme un vrai Sampler.
				if proj.Channels[currentChannel].Kind != KindAutomation && proj.Channels[currentChannel].Kind != KindLayer {
					proj.Channels[currentChannel].Kind = KindSampler
				}
			}

		case idPatternNew:
			// D'apres la doc communautaire, cet evenement apparait deux fois
			// par pattern (une fois pour la section "notes", une fois pour
			// la section "metadonnees/nom") — on fusionne les deux par
			// index de pattern plutot que de creer deux entrees.
			idx := int(binary.LittleEndian.Uint16(value))
			if existing, ok := patternByIdx[idx]; ok {
				currentPattern = existing
			} else {
				currentPattern = len(proj.Patterns)
				patternByIdx[idx] = currentPattern
				proj.Patterns = append(proj.Patterns, Pattern{Index: idx})
			}

		case idPatternName:
			if currentPattern >= 0 && currentPattern < len(proj.Patterns) {
				proj.Patterns[currentPattern].Name = decodeText(value, useUnicode)
			}

		case idPatternNotes:
			if currentPattern >= 0 && currentPattern < len(proj.Patterns) {
				notes, err := parseNotes(value)
				if err != nil {
					return nil, fmt.Errorf("parsing des notes du pattern %d: %w", currentPattern, err)
				}
				proj.Patterns[currentPattern].Notes = append(proj.Patterns[currentPattern].Notes, notes...)
			}

		case idProjectTempo:
			raw := binary.LittleEndian.Uint32(value)
			proj.TempoBPM = float64(raw) / 1000.0
		}
	}

	return proj, nil
}

// readVarInt lit un entier de taille variable (encodage LEB128 non signe,
// tel qu'utilise par la lib "construct" en Python). Retourne la valeur et
// le nombre d'octets consommes.
func readVarInt(data []byte) (value int, consumed int, err error) {
	shift := 0
	for {
		if consumed >= len(data) {
			return 0, 0, errors.New("fin de fichier inattendue pendant la lecture d'un varint")
		}
		b := data[consumed]
		consumed++
		value |= int(b&0x7f) << shift
		if b&0x80 == 0 {
			break
		}
		shift += 7
		if shift > 63 {
			return 0, 0, errors.New("varint trop long")
		}
	}
	return value, consumed, nil
}

// decodeText decode un evenement texte en UTF-16LE (projets recents) ou en
// ASCII/Latin-1 (tres vieux projets), en retirant le(s) caractere(s) nul de
// terminaison.
func decodeText(raw []byte, unicode bool) string {
	if !unicode {
		return strings.TrimRight(string(raw), "\x00")
	}
	if len(raw)%2 != 0 {
		raw = raw[:len(raw)-1]
	}
	runes := make([]uint16, 0, len(raw)/2)
	for i := 0; i+1 < len(raw); i += 2 {
		u := binary.LittleEndian.Uint16(raw[i : i+2])
		if u == 0 {
			break
		}
		runes = append(runes, u)
	}
	return utf16ToString(runes)
}

func utf16ToString(u []uint16) string {
	// Decodage UTF-16 minimal (suffisant pour les caracteres du BMP, qui
	// couvrent l'immense majorite des noms de channels/samples reels).
	var sb strings.Builder
	for _, r := range u {
		sb.WriteRune(rune(r))
	}
	return sb.String()
}

// filenameFromWindowsPath extrait le nom de fichier d'un chemin Windows
// (les .flp stockent toujours des chemins avec des antislashs, meme si le
// fichier est lu sur une autre plateforme).
func filenameFromWindowsPath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	parts := strings.Split(p, "/")
	return parts[len(parts)-1]
}

// flVersionIsUnicode determine si les evenements texte du fichier utilisent
// l'UTF-16LE (FL Studio >= 11.5) ou l'ASCII simple (versions plus anciennes).
func flVersionIsUnicode(version string) bool {
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return true // par defaut, hypothese la plus probable pour un projet recent
	}
	major := parseIntSafe(parts[0])
	minor := parseIntSafe(parts[1])
	if major > 11 {
		return true
	}
	if major == 11 && minor >= 5 {
		return true
	}
	return major < 11 // valeurs non concluantes -> ASCII par securite pour les tres vieilles versions
}

func parseIntSafe(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// noteRecordSize est la taille en octets d'un enregistrement de note dans
// l'evenement PatternID.Notes (voir structure dans pyflp/pattern.py).
const noteRecordSize = 24

func parseNotes(data []byte) ([]Note, error) {
	if len(data)%noteRecordSize != 0 {
		return nil, fmt.Errorf("taille inattendue pour un bloc de notes: %d (attendu multiple de %d)", len(data), noteRecordSize)
	}
	count := len(data) / noteRecordSize
	notes := make([]Note, 0, count)
	for i := 0; i < count; i++ {
		b := data[i*noteRecordSize : (i+1)*noteRecordSize]
		notes = append(notes, Note{
			Position:    binary.LittleEndian.Uint32(b[0:4]),
			RackChannel: binary.LittleEndian.Uint16(b[6:8]),
			Length:      binary.LittleEndian.Uint32(b[8:12]),
			Key:         binary.LittleEndian.Uint16(b[12:14]),
			Pan:         b[20],
			Velocity:    b[21],
		})
	}
	return notes, nil
}

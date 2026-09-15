// Package midiwriter ecrit des fichiers MIDI standard (.mid, format SMF 0)
// a partir des notes extraites d'un pattern .flp. Implementation "from
// scratch" en pure Go, sans dependance externe.
package midiwriter

import (
	"bytes"
	"encoding/binary"
	"os"

	"flpmine/flp"
)

// defaultNoteLength est la duree (en ticks) utilisee pour les notes dont la
// longueur vaut 0 dans le fichier .flp — frequent pour les hits de
// batterie declenches via le step sequencer, qui n'ont pas de "longueur"
// au sens piano-roll. Sans ca, la note serait inaudible/invisible une fois
// exportee en MIDI standard.
const defaultNoteLength = 60 // en ticks ; a 96 PPQ, ~ une double-croche

// WritePattern ecrit les notes d'un pattern dans un fichier .mid (SMF 0,
// une seule piste). ppq est la resolution (pulses/ticks par noire) du
// projet source, reutilisee telle quelle comme division du fichier MIDI —
// FL Studio utilise deja la meme unite, donc position/duree des notes sont
// directement compatibles, aucune conversion necessaire.
func WritePattern(path string, ppq uint16, notes []flp.Note, tempoBPM float64) error {
	var track bytes.Buffer

	type event struct {
		tick    uint32
		isNoteOn bool
		note    uint8
		velocity uint8
	}

	var events []event

	if tempoBPM > 0 {
		// evenement de tempo place au tout debut (gere separement, avant
		// le tri des notes, puisqu'il n'a pas de "note").
	}

	for _, n := range notes {
		length := n.Length
		if length == 0 {
			length = defaultNoteLength
		}
		note := n.Key
		if note > 127 {
			note = 127
		}
		vel := n.Velocity
		if vel == 0 {
			vel = 100 // une note a velocite 0 serait silencieuse/ignoree en MIDI standard
		}
		events = append(events, event{n.Position, true, uint8(note), vel})
		events = append(events, event{n.Position + length, false, uint8(note), 0})
	}

	// Tri stable par tick ; a tick egal, les Note Off passent avant les
	// Note On (evite de couper une note qui redemarre au meme instant).
	for i := 1; i < len(events); i++ {
		for j := i; j > 0; j-- {
			a, b := events[j-1], events[j]
			swap := a.tick > b.tick || (a.tick == b.tick && a.isNoteOn && !b.isNoteOn)
			if !swap {
				break
			}
			events[j-1], events[j] = events[j], events[j-1]
		}
	}

	var lastTick uint32

	// Meta-evenement de tempo, tick 0.
	if tempoBPM > 0 {
		writeVLQ(&track, 0)
		microsPerQuarter := uint32(60000000.0 / tempoBPM)
		track.Write([]byte{0xFF, 0x51, 0x03,
			byte(microsPerQuarter >> 16), byte(microsPerQuarter >> 8), byte(microsPerQuarter)})
	}

	for _, ev := range events {
		delta := ev.tick - lastTick
		lastTick = ev.tick
		writeVLQ(&track, delta)
		if ev.isNoteOn {
			track.Write([]byte{0x90, ev.note, ev.velocity})
		} else {
			track.Write([]byte{0x80, ev.note, 0})
		}
	}

	// Fin de piste.
	writeVLQ(&track, 0)
	track.Write([]byte{0xFF, 0x2F, 0x00})

	var out bytes.Buffer
	out.WriteString("MThd")
	binary.Write(&out, binary.BigEndian, uint32(6))
	binary.Write(&out, binary.BigEndian, uint16(0)) // format 0 : une seule piste
	binary.Write(&out, binary.BigEndian, uint16(1)) // ntrks
	binary.Write(&out, binary.BigEndian, ppq)        // division = PPQ du projet source

	out.WriteString("MTrk")
	binary.Write(&out, binary.BigEndian, uint32(track.Len()))
	out.Write(track.Bytes())

	return os.WriteFile(path, out.Bytes(), 0644)
}

// writeVLQ ecrit un entier en Variable Length Quantity, l'encodage des
// delta-times utilise par le format MIDI standard (poids fort en premier,
// bit de continuation sur tous les octets sauf le dernier — attention,
// c'est un ordre DIFFERENT du VarInt utilise dans le format .flp).
func writeVLQ(buf *bytes.Buffer, value uint32) {
	var stack [5]byte
	i := len(stack)
	i--
	stack[i] = byte(value & 0x7F)
	value >>= 7
	for value > 0 {
		i--
		stack[i] = byte(value&0x7F) | 0x80
		value >>= 7
	}
	buf.Write(stack[i:])
}

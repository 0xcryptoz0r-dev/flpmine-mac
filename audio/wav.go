// Package audio lit des fichiers WAV (PCM) et en extrait des
// caracteristiques simples (duree, energie par bande de frequence, taux de
// passage par zero) pour verifier qu'un son correspond a son tag suppose.
//
// Volontairement limite au format WAV (PCM 16/24/32 bits, mono/stereo) :
// c'est le format quasi-exclusif des packs de samples utilises en
// production (kicks, snares, 808, hihats...). Le MP3 (ex: tags vocaux) et
// les formats compresses ne sont pas geres — un decodeur MP3 correct est un
// projet a part entiere, hors scope pour cette verification "sanity check".
package audio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
)

// Samples represente un signal audio mono, normalise entre -1 et 1.
type Samples struct {
	Data       []float64
	SampleRate int
}

func (s Samples) DurationSeconds() float64 {
	if s.SampleRate == 0 {
		return 0
	}
	return float64(len(s.Data)) / float64(s.SampleRate)
}

// ReadWAV lit un fichier .wav et retourne son signal en mono.
func ReadWAV(path string) (Samples, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Samples{}, fmt.Errorf("lecture du fichier: %w", err)
	}
	return parseWAV(data)
}

func parseWAV(data []byte) (Samples, error) {
	if len(data) < 44 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return Samples{}, errors.New("pas un fichier WAV valide (RIFF/WAVE manquant)")
	}

	var (
		sampleRate    int
		numChannels   int
		bitsPerSample int
		audioFormat   int
		samplesRaw    []byte
	)

	pos := 12
	for pos+8 <= len(data) {
		chunkID := string(data[pos : pos+4])
		chunkSize := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		chunkStart := pos + 8
		if chunkStart+chunkSize > len(data) {
			chunkSize = len(data) - chunkStart // fichier tronque : on prend ce qui reste
		}

		switch chunkID {
		case "fmt ":
			if chunkSize < 16 {
				return Samples{}, errors.New("chunk fmt trop petit")
			}
			fmtChunk := data[chunkStart : chunkStart+chunkSize]
			audioFormat = int(binary.LittleEndian.Uint16(fmtChunk[0:2]))
			numChannels = int(binary.LittleEndian.Uint16(fmtChunk[2:4]))
			sampleRate = int(binary.LittleEndian.Uint32(fmtChunk[4:8]))
			bitsPerSample = int(binary.LittleEndian.Uint16(fmtChunk[14:16]))
		case "data":
			samplesRaw = data[chunkStart : chunkStart+chunkSize]
		}

		pos = chunkStart + chunkSize
		if chunkSize%2 == 1 {
			pos++ // padding sur les tailles de chunk impaires
		}
	}

	if sampleRate == 0 || numChannels == 0 || bitsPerSample == 0 || samplesRaw == nil {
		return Samples{}, errors.New("chunks fmt/data manquants ou incomplets")
	}
	// 3 = IEEE float, 1 = PCM entier. Les autres (ex: 6=ALaw, 7=MULaw,
	// ou WAVE_FORMAT_EXTENSIBLE=0xFFFE) ne sont pas geres ici.
	if audioFormat != 1 && audioFormat != 3 && audioFormat != 0xFFFE {
		return Samples{}, fmt.Errorf("format audio non supporte (code %d, seul PCM/IEEE float est gere)", audioFormat)
	}

	bytesPerSample := bitsPerSample / 8
	if bytesPerSample == 0 {
		return Samples{}, errors.New("bitsPerSample invalide")
	}
	frameSize := bytesPerSample * numChannels
	if frameSize == 0 {
		return Samples{}, errors.New("taille de frame invalide")
	}
	numFrames := len(samplesRaw) / frameSize

	mono := make([]float64, numFrames)
	for i := 0; i < numFrames; i++ {
		var sum float64
		for ch := 0; ch < numChannels; ch++ {
			off := i*frameSize + ch*bytesPerSample
			sum += readSample(samplesRaw[off:off+bytesPerSample], bitsPerSample, audioFormat)
		}
		mono[i] = sum / float64(numChannels)
	}

	return Samples{Data: mono, SampleRate: sampleRate}, nil
}

func readSample(b []byte, bits int, format int) float64 {
	switch {
	case format == 3 && bits == 32: // IEEE float 32 bits
		bits32 := binary.LittleEndian.Uint32(b)
		return float64(math.Float32frombits(bits32))
	case bits == 16:
		v := int16(binary.LittleEndian.Uint16(b))
		return float64(v) / 32768.0
	case bits == 24:
		v := int32(b[0]) | int32(b[1])<<8 | int32(b[2])<<16
		if v&0x800000 != 0 {
			v |= -0x1000000 // extension du signe sur 24 -> 32 bits
		}
		return float64(v) / 8388608.0
	case bits == 32:
		v := int32(binary.LittleEndian.Uint32(b))
		return float64(v) / 2147483648.0
	case bits == 8:
		return (float64(b[0]) - 128) / 128.0
	default:
		return 0
	}
}

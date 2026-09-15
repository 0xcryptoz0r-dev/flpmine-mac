package audio

import "math"

// Features regroupe les caracteristiques utilisees pour verifier qu'un son
// correspond a son tag suppose. Ce sont des indicateurs volontairement
// simples (pas de FFT complete) mais suffisants pour distinguer les grandes
// familles de sons percussifs/mélodiques d'un pack de production.
type Features struct {
	DurationSeconds  float64
	LowEnergyRatio   float64 // part de l'energie sous ~150 Hz (kicks, 808)
	MidEnergyRatio   float64 // part de l'energie entre ~150 Hz et ~2500 Hz (snare, clap, voix)
	HighEnergyRatio  float64 // part de l'energie au-dessus de ~2500 Hz (hihat, cymbale)
	ZeroCrossingRate float64 // passages par zero / seconde (eleve = bruit/percussif brillant, faible = tonal grave)
	PeakAmplitude    float64
}

// Extract calcule les caracteristiques d'un signal. analyse au maximum les
// 800 premieres millisecondes (suffisant pour un hit percussif ou un
// oneshot ; au-dela, les fichiers longs/loops sont tronques pour la vitesse
// de calcul sans perdre l'information la plus pertinente, qui est toujours
// au debut du son).
func Extract(s Samples) Features {
	data := s.Data
	maxSamples := int(0.8 * float64(s.SampleRate))
	if maxSamples > 0 && len(data) > maxSamples {
		data = data[:maxSamples]
	}
	if len(data) == 0 || s.SampleRate == 0 {
		return Features{}
	}

	lowCutoff := 150.0
	midCutoff := 2500.0
	alphaLow := onePoleAlpha(lowCutoff, s.SampleRate)
	alphaMid := onePoleAlpha(midCutoff, s.SampleRate)

	low := lowPass(data, alphaLow)
	remainder := subtract(data, low)
	mid := lowPass(remainder, alphaMid)
	high := subtract(remainder, mid)

	lowE := rmsEnergy(low)
	midE := rmsEnergy(mid)
	highE := rmsEnergy(high)
	total := lowE + midE + highE
	if total == 0 {
		total = 1 // evite une division par zero sur un fichier silencieux
	}

	zcr := zeroCrossingRate(data, s.SampleRate)
	peak := peakAmplitude(data)

	return Features{
		DurationSeconds:  s.DurationSeconds(),
		LowEnergyRatio:   lowE / total,
		MidEnergyRatio:   midE / total,
		HighEnergyRatio:  highE / total,
		ZeroCrossingRate: zcr,
		PeakAmplitude:    peak,
	}
}

// onePoleAlpha calcule le coefficient d'un filtre passe-bas a un pole pour
// une frequence de coupure approximative donnee.
func onePoleAlpha(cutoffHz float64, sampleRate int) float64 {
	rc := 1.0 / (2 * math.Pi * cutoffHz)
	dt := 1.0 / float64(sampleRate)
	return dt / (rc + dt)
}

func lowPass(data []float64, alpha float64) []float64 {
	out := make([]float64, len(data))
	if len(data) == 0 {
		return out
	}
	out[0] = data[0]
	for i := 1; i < len(data); i++ {
		out[i] = out[i-1] + alpha*(data[i]-out[i-1])
	}
	return out
}

func subtract(a, b []float64) []float64 {
	out := make([]float64, len(a))
	for i := range a {
		out[i] = a[i] - b[i]
	}
	return out
}

func rmsEnergy(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	var sum float64
	for _, v := range data {
		sum += v * v
	}
	return math.Sqrt(sum / float64(len(data)))
}

func zeroCrossingRate(data []float64, sampleRate int) float64 {
	if len(data) < 2 {
		return 0
	}
	crossings := 0
	for i := 1; i < len(data); i++ {
		if (data[i-1] >= 0) != (data[i] >= 0) {
			crossings++
		}
	}
	seconds := float64(len(data)) / float64(sampleRate)
	if seconds == 0 {
		return 0
	}
	return float64(crossings) / seconds
}

func peakAmplitude(data []float64) float64 {
	var peak float64
	for _, v := range data {
		a := math.Abs(v)
		if a > peak {
			peak = a
		}
	}
	return peak
}

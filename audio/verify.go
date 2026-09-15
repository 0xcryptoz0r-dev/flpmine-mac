package audio

import "fmt"

// VerifyResult est le resultat de la verification d'un son par rapport a
// son tag suppose (donne par le nom du channel/fichier).
type VerifyResult struct {
	Tag        string
	Features   Features
	Mismatch   bool
	Reason     string // vide si pas de probleme detecte
}

// Verify compare les caracteristiques audio extraites a ce qu'on attend
// pour le tag donne, et signale une INCOHERENCE FORTE si elles ne
// correspondent manifestement pas.
//
// Volontairement conservateur : ce n'est pas un classifieur audio complet
// (il ne cherche pas a deviner le "bon" tag tout seul), seulement un
// detecteur d'anomalies flagrantes — ex: un channel nomme "Kick" dont le
// fichier audio est en realite un hi-hat. Les cas ambigus ne sont pas
// signales, pour eviter les faux positifs. Les seuils sont une premiere
// estimation ; ils gagneront a etre affines avec de vrais exemples de ta
// bibliotheque.
func Verify(tag string, f Features) VerifyResult {
	res := VerifyResult{Tag: tag, Features: f}

	switch tag {
	case "kick", "808":
		if f.HighEnergyRatio > 0.5 {
			res.Mismatch = true
			res.Reason = fmt.Sprintf("tague %q mais le son est tres domine par les hautes frequences (%.0f%%) — ressemble plutot a un hi-hat/cymbale", tag, f.HighEnergyRatio*100)
		}

	case "hihat", "open_hat", "crash", "cymbal", "shaker":
		if f.LowEnergyRatio > 0.55 {
			res.Mismatch = true
			res.Reason = fmt.Sprintf("tague %q mais le son est tres domine par les basses frequences (%.0f%%) — ressemble plutot a un kick/808", tag, f.LowEnergyRatio*100)
		}

	case "snare", "clap", "rim", "perc":
		if f.LowEnergyRatio > 0.65 && f.HighEnergyRatio < 0.08 {
			res.Mismatch = true
			res.Reason = fmt.Sprintf("tague %q mais le son n'a presque aucune haute frequence (%.0f%% basses / %.0f%% hautes) — ressemble plutot a un kick/808", tag, f.LowEnergyRatio*100, f.HighEnergyRatio*100)
		}
	}

	return res
}

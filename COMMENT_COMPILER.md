# Compiler FlpMine pour Mac (aucune ligne de commande requise)

Ce dossier contient le code de FlpMine pour Mac. Il n'y a pas de fichier
`.app` dedans — c'est GitHub qui va le compiler pour toi, gratuitement,
sur une vraie machine Mac dans le cloud. Suis ces etapes une fois, dans
l'ordre.

## 1. Creer un compte GitHub (si tu n'en as pas)

Va sur https://github.com/signup, cree un compte gratuit.

## 2. Creer un nouveau depot (repository)

- En haut a droite du site, clique sur le **+** puis **New repository**.
- Nom : `flpmine-mac` (ou ce que tu veux).
- Laisse-le en **Public** (le plan gratuit de compilation Mac est plus
  simple avec un depot public).
- Ne coche aucune case (pas de README, pas de .gitignore).
- Clique **Create repository**.

## 3. Envoyer ce dossier dans le depot

Sur la page qui s'affiche, cherche le lien **"uploading an existing
file"** (ou "Add file" → "Upload files" en haut du depot).

- Glisse **tout le contenu de ce dossier** (pas le dossier lui-meme,
  son contenu : `go.mod`, les dossiers `flp`, `classifier`, `audio`,
  `midiwriter`, `cmd`, `.github`) dans la zone d'upload.
- En bas de page, clique **Commit changes**.

⚠️ Le dossier `.github` est cache par defaut sur certains systemes.
Si tu ne le vois pas dans ton explorateur de fichiers, active
l'affichage des fichiers/dossiers caches (sur Mac : `Cmd + Maj + .`
dans le Finder).

## 4. Lancer la compilation

- En haut du depot, clique sur l'onglet **Actions**.
- Tu devrais voir "Build FlpMine for macOS" dans la liste a gauche —
  clique dessus.
- Un bouton **"Run workflow"** apparait a droite → clique dessus, puis
  a nouveau sur le bouton vert **"Run workflow"** qui apparait.
- Attends 2-3 minutes (la page se rafraichit automatiquement, un rond
  jaune tourne pendant que ca compile, il devient vert quand c'est fini).

## 5. Telecharger l'application

- Une fois le rond devenu vert, clique sur le run termine.
- Tout en bas de la page, section **Artifacts** : tu verras deux
  fichiers a telecharger :
  - `FlpMine-apple-silicon` → pour les Mac M1/M2/M3/M4 (les plus
    recents)
  - `FlpMine-intel` → pour les Mac plus anciens avec processeur Intel
- Pas sur lequel choisir ? Menu Pomme → "A propos de ce Mac" → regarde
  la ligne "Puce" ou "Processeur".
- Telecharge le bon zip, decompresse-le : tu obtiens `FlpMine.app`.

## 6. Premier lancement

macOS bloque par defaut les applications qui ne viennent pas de l'App
Store ou d'un developpeur identifie Apple (meme si l'app n'a rien de
dangereux — c'est juste qu'elle n'est pas "signee" officiellement, ce
qui coute cher et n'a pas de rapport avec la securite reelle du code).

Pour l'autoriser :
- Clic droit (ou Ctrl+clic) sur `FlpMine.app` → **Ouvrir**.
- Une fenetre d'avertissement apparait → clique a nouveau **Ouvrir**.
- Les fois suivantes, un simple double-clic suffira.

## Si la compilation echoue (rond rouge au lieu de vert)

Clique sur le run rouge, puis sur "build" pour voir le detail de
l'erreur, et copie-colle moi ce que tu vois — je corrige le code et tu
n'auras qu'a re-uploader les fichiers modifies.

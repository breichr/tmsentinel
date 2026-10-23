# tmsentinel — Roadmap und Gates (04)

Jede Aufgabe gehört zu genau einer Phase. Ein Gate muss bestanden sein, bevor Arbeit an der nächsten Phase beginnt. Gates scheitern zu dürfen ist der Sinn der Gates.

## Phase 0 — Beweis (1 Nachmittag bis 2 Tage)

Ziel: der FDA-freie Kern funktioniert auf dem eigenen Mac.

- Prototyp, eine Go-Datei: `log show --style ndjson --last 7d --predicate 'subsystem == "com.apple.TimeMachine"'`, Start/Abschluss-Ereignisse gegenüberstellen, Alter des letzten Erfolgs ausgeben.
- Verifizieren: `tmutil latestbackup` tatsächlich FDA-gebunden auf dem eigenen System; `destinationinfo` und `status` ohne FDA.
- Korpus-Start: `export`-Kommando roh hacken, zwei Wochen reale Backups mitschneiden, mindestens einen SMB-Fehler erzwingen (NAS-Konfiguration verdrehen).

Gate 1: Liefert der Unified Log zuverlässig Start und Abschluss jedes Backups ohne FDA? Nein → Ursache klären; wenn unlösbar, Idee verwerfen — der gesamte FDA-freie Differenzierungswert steht auf diesem Fundament.

## Phase 1 — MVP: `check`, `destinations` (Woche 1–2)

- Beide Subcommands, Exit-Codes, `--json`, Defaults ohne Konfiguration.
- Homebrew-Tap (eigener), GitHub Releases, README mit dem 26.4-SMB-Vorfall als Referenzfall.
- Zwei Wochen Eigenbetrieb parallel zum Konkurrenz-Test: tmwatch und das healthchecks-Skript laufen mit, Schwächenliste führen.

Gate 2: Nutzt du es selbst nach zwei Wochen täglich? Deckt es mindestens eine Situation ab, die tmwatch und das Skript nicht abdecken (z. B. laufender Backup, Ziel-Offline vs. Staleness-Unterscheidung)? Nein → vor dem Ausbau ernsthaft prüfen, ob das Tool existieren muss.

## Phase 2 — Alarmierung: `notify` (Woche 3–4)

- healthchecks.io-Ping (Exit-Code-Suffix, Body mit letztem Zustand), macOS-Notification via osascript (nur bei Zustandswechsel, nicht bei jedem Lauf — Zustandswechsel aus dem Log ableitbar, bleibt zustandslos).
- LaunchAgent-Sample (`com.user.tmsentinel.plist`, StartInterval 300) mit Install- und Deinstall-Skript.
- Konfigurationsdatei: Schwelle, UUID, Kanal.

Gate 3: Läuft der LaunchAgent eine Woche störungsfrei? Bekommt die healthchecks-Seite die Zustände korrekt?

## Phase 3 — Diagnose: `why` (Woche 5–8)

- Fingerabdruck-Katalog aus dem Korpus, erste fünf Muster (siehe Tabelle), jede mit Fixture und Test.
- `docs/reasons/`-Seiten, verlinkt aus der Diagnose-Ausgabe.
- `selftest` als Erstversion.

Start-Katalog:

| Muster (Signaturen im Log) | Klartext-Diagnose |
| --- | --- |
| SMB-Session-Abbruch, Signing-Fehler | Ziel-Verbindung brach ab — bekannte SMB-Regression? NAS-Firmware prüfen |
| Sparsebundle/DMG-Mount-Fehler | Backup-Image konnte nicht gemountet werden — Ziel prüfen |
| `BackupDispatching`-Start ohne Abschluss-Ereignis | Backup startete, brach aber ab oder hängt seit X |
| Kein einziges Start-Ereignis im Fenster | Time Machine hat gar nicht versucht zu sichern — Ziel oder Konfiguration? |
| Thinning-/Snapshot-Fehler | Speicherbereinigung schlug fehl — Ziel voll? |

Gate 4: Erkennt `why` im Eigenbetrieb jede reale Störung korrekt? Eine falsche Diagnose ist schlimmer als keine — besser knapp und korrekt als vollständig und rätiglich.

## Phase 4 — Reifung und Verteilung (ab Woche 9, parallel)

- Homebrew-core-Einreichung (Voraussetzung: stabile Exit-Codes, einen Monat Betrieb, einige Nutzer).
- macOS-27-Beta-Track: Fixtures gegen die Beta, sobald verfügbar.
- CONTRIBUTING: wie man einen neuen Fingerabdruck mit Fixture einreicht.
- Optional, nur bei Traktion: zweite Ziele je Backup, `latestbackup`-Tiefe hinter FDA-Flag.

## Abbruchkriterien (gesamt)

- Gate 1 scheitert: fundamentales Fundament fehlt.
- Gate 2 scheitert: selbst genutztes Werkzeug ist das stärkste Validierungssignal.
- Oakley publiziert ein CLI-Äquivalent mit derselben FDA-freien Diagnose-Leistung: beitragen statt konkurrieren, falls die Lücke real bleibt.

## Wartungsmodell (der eigentliche Kostenpunkt)

| Termin | Arbeit |
| --- | --- |
| macOS-Beta (Juni–Sept) | Fixtures gegen Beta, `selftest` gegen Drift, Kompatibilitäts-Branch |
| Release (Sept/Okt) | Smoke-Test, Patch-Release, Versions-Matrix im README pflegen |
| Laufend | CI-Fixture-Sätze je unterstützter Version, 2–4 Tage/Jahr Budget gesamt |

Bewusst ein Projekt starten statt drei. Erweiterungen (BTM-Auditor, Signatur-Auditor) erst ansetzen, wenn tmsentinel einen Release-Zyklus überlebt hat.

## Erfolgsmessung

Primär: Eigennutzung überlebt drei Monate. Sekundär: brew-Installationen, Fingerabdruck-Contributions, Issue-Qualität (Fehlerberichte mit `export`-Anhängen). Sterne sind Vanity; Installationen mit Rückmeldungen sind Substanz.

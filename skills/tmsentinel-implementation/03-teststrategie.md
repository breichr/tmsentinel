# tmsentinel — Teststrategie (03)

## Fixtures

- anonymisierte, reale `log show`-Exports in `testdata/`: gute Backups, SMB-Abbruch, Ziel offline, laufender Backup, hängender Backup (Start ohne Abschluss).
- `tmsentinel export` erzeugt die Fixtures; Tests laufen gegen die Rohdateien als Golden-Files.
- Fixtures werden nie von Hand erfunden, wenn ein realer Export existiert. Erfundene Fixtures sind erlaubt für Randfälle, müssen im Header als `synthetic` markiert sein.
- Anonymisierung vor jedem Commit: Hostnamen, Volume-Namen, Netzwerkpfade, Benutzerkürzel ersetzen. Ein Skript `testdata/anonymize.sh` ist Teil des Repos.

## Parser-Drift

- Jede unterstützte macOS-Version (Sequoia, Tahoe, 27 sobald verfügbar) bekommt einen Fixture-Satz.
- Ein CI-Job markiert Parser-Drift: läuft ein Fixture-Satz gegen den Parser und ändert sich das Ergebnis gegenüber der Golden-File, schlägt der Build fehl.
- Neue macOS-Version: erst Fixtures von einem realen System aufnehmen, dann Parser anpassen. Niemals den Parser auf Verdacht ändern.

## selftest

- `tmsentinel selftest` prüft auf dem Live-System, ob die erwarteten Felder und Ereignistypen noch kommen — der Nutzer merkt Schema-Brüche durch eine klare Meldung statt stiller Fehlfunktion.
- Implementierung: kurzes `log show`-Fenster (1 h), Prüfung auf Pflichtfelder und mindestens ein bekanntes Ereignis-Muster. Ausgabe: Liste der geprüften Schnittstellen mit ok/fail.
- selftest ist bewusst kein Unit-Test-Ersatz, sondern ein Live-Drift-Sensor für Nutzer und für den Beta-Track (Juni–September).

## Zeitfenster-Logik

- deterministische Fake-Uhren, keine echte Zeit in Tests. `freshness` nimmt eine Uhr als Abhängigkeit, kein direktes `time.Now()` im Paket.
- Schwellengrenzfälle (exakt 48 h, 48 h − 1 s, laufender Backup am Schwellenrand) sind Pflicht-Testfälle.

## Regeln

- Kein Test führt `log`, `tmutil` oder Netzwerkaufrufe aus — Subprozess-Aufrufe werden hinter Schnittstellen gemockt, geparste echte Ausgaben als Fixtures aufgezeichnet.
- Jeder Fingerabdruck im `reason`-Katalog hat einen Fixture-Test, der die Klartext-Diagnose gegen die Golden-File prüft.
- `go test ./...` muss ohne Netzwerk und ohne macOS-spezifische Rechte in CI laufen (parser-Tests sind plattformunabhängig; smoke-artige Tests dürfen `//go:build darwin` tragen).

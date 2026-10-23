# tmsentinel — Spezifikation (01)

Produkt: Go-CLI, das beantwortet: Läuft mein Time-Machine-Backup wirklich — und wenn nicht, warum nicht? Betrieb ohne Full Disk Access, ohne Daemon, ohne GUI. Angetrieben von launchd oder cron; jede Ausführung ist unabhängig.

Zielgruppen: Solo-Admins, Homelab-Betreiber, Entwickler mit headless Macs, Nutzer von healthchecks.io und vergleichbaren Dead-Man-Switches.

## CLI-Oberfläche

```
tmsentinel check          Frische prüfen, Exit-Code für Scheduler
tmsentinel check --json   Maschinenlesbar
tmsentinel why            Ursache des letzten fehlgeschlagenen oder hängenden Backups
tmsentinel destinations   Ziele, Typ, letzter Stand je Ziel
tmsentinel notify         check + Alarmierung (healthchecks-Ping, macOS-Notification)
tmsentinel export         Rohdaten-Export für Debugging und Test-Fixtures
tmsentinel selftest       Schnittstellen-Gesundheit prüfen (Schema-Drift-Erkennung)
tmsentinel version
```

Nicht-Ziele (bewusst, jedes ist ein besetztes Nachbarsegment): kein Scheduling eigener Backups (TimeMachineEditor, Backup Scheduler), keine GUI (T2M2, Menüleiste), kein TUI, keine Flotten-Features (Watchman Monitoring), kein History-Browser (BackupLoupe).

## Exit-Codes (stabil, dokumentierter Vertrag)

| Code | Bedeutung |
| --- | --- |
| 0 | Letzter Backup erfolgreich und frisch (innerhalb Schwelle) |
| 1 | Interner Fehler |
| 2 | Letzter Backup älter als Schwelle (Standard 48 h) |
| 3 | Kein Ziel konfiguriert |
| 4 | Ziel konfiguriert, aber nicht erreichbar |
| 5 | Backup läuft gerade (gilt als gesund, Exit 0 mit Hinweis) |

Regeln:

- Codes 0–5 niemals umbuchen oder umnummerieren. Neue Zustände bekommen neue Codes ab 6, nur mit Major-Version.
- Exit 5 gilt laut Tabelle als Hinweis; `check` beendet sich in diesem Fall mit 0 und gibt den Hinweis im Text aus. Der Code 5 bleibt für `--json`-Verbraucher reserviert, die den Zustand explizit abfragen wollen (Feld `running: true`).
- Exit 1 nur für Fehler im Tool selbst (Parse-Fehler, Subprozess-Absturz), nie für Backup-Probleme.

## Konfiguration

`~/.config/tmsentinel/config.toml` — Schwelle, healthchecks-UUID, Notification-Kanal, optionale FDA-Tiefe.

- Keine Pflichtkonfiguration für `check`: sinnvolle Defaults, sofort lauffähig nach `brew install`.
- Defaults: Schwelle 48 h, Notification aus, kein healthchecks-Ping.
- Fehlende oder defekte Konfigurationsdatei: Defaults verwenden und im `--json`-Ausgang vermerken, niemals mit Exit 1 abbrechen.

## Ausgabeformat

- Textmodus: kurz, ein Ergebnis pro Zeile, menschenlesbar; `why` liefert Klartext-Diagnose plus Link auf die versionierte Erklärung in `docs/reasons/`.
- `--json`: stabiles Schema, additive Felder nur; niemals Felder umbenennen oder entfernen. Struktur: `{ state, last_success_age_h, threshold_h, destinations[], diagnosis{}, running, config_loaded }`.

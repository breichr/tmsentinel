# tmsentinel — Architektur (02)

## Grundprinzip: zustandslos

Der Zustand "letzter erfolgreicher Backup" wird bei jedem Lauf aus dem Unified Log abgeleitet, nicht in einer eigenen Datenbank gehalten. Ein zweiter Lauf ändert nichts — das macht Tests, Cron-Einbindung und Fehlersuche trivial. Zustandswechsel-Erkennung (für Notification bei Zustandswechsel) wird ebenfalls aus dem Log abgeleitet, nicht aus gespeichertem Zustand.

## Datenquellen mit Fallback-Kette

| Quelle | Zweck | FDA nötig |
| --- | --- | --- |
| `log show` — Subsystem `com.apple.TimeMachine` | Backup-Start/Abschluss/Fehler-Ereignisse, letzter Erfolg | Nein |
| `tmutil destinationinfo` | Ziele, Typ (lokal/SMB/APFS) | Nein |
| `tmutil status` | Läuft gerade ein Backup | Nein |
| `log stream --style json` (nur `notify`-Modus, optional) | Live-Tail für sofortige Alarme | Nein |
| `tmutil latestbackup`, `listbackups` | Optionale Tiefe, wenn FDA ohnehin vorliegt | Ja, optional |

Fallback-Kette: Unified Log zuerst; fällt er aus, greifen `tmutil`-Quellen mit geringerer Aussagekraft; fällt alles aus, Exit 1 mit klarer Meldung, welche Quelle versagte.

## Module (Paketgrenzen)

| Paket | Verantwortung |
| --- | --- |
| `event` | Unified-Log-Parser, Ereignis-Modell (Start/Abschluss/Fehler) |
| `state` | Backup-Zustandsmaschine: idle → running → success/failed |
| `freshness` | Alterslogik, Schwellen |
| `reason` | Fehler-Fingerabdrücke → Klartext-Diagnose |
| `notify` | healthchecks-Ping, macOS-Notification via osascript, Exit-Code-Adapter |
| `cli` | Subcommand-Dispatch, Ausgabe, Konfiguration |

Regel: `event`, `state`, `freshness`, `reason` sind reine Logik ohne Prozessausführung. Nur `event` (für den Log-Subprozess) und `notify` starten Subprozesse. Nichts sonst macht I/O — das hält alles testbar gegen Fixtures.

## Technische Kernentscheidungen

1. `log show` als Subprozess statt OSLogStore-API: funktioniert aus launchd ohne GUI-Sitzung, kein Swift-Mischbau, FDA-frei. Der Preis ist die Startlaufzeit von `log show`; verkraftbar, weil tmsentinel nur alle paar Minuten läuft.
2. Polling statt Streaming-Daemon: launchd StartInterval (alle 5 Minuten) auf `tmsentinel notify`. Kein Resident-Prozess, kein Memory-Leak-Risiko. Ein mitgeliefertes LaunchAgent-Sample macht die Installation zu einem Einzeiler.
3. `--style ndjson` bei `log show` (`stream` kennt kein ndjson — nur `show`). Parse robust gegen fehlende Felder, logge ungeparste Datensätze mit.
4. Schreibender Zugriff: keiner (siehe Invarianten in SKILL.md).

## Standard-Abfragen

Prüfpredikat für alle Log-Abfragen:

```
log show --style ndjson --last 7d --predicate 'subsystem == "com.apple.TimeMachine"'
```

- Fenster 7 Tage als Default; konfigurierbar, aber nie unter 24 h (Schwellenlogik braucht Vorlauf).
- `event` abstrahiert das Fenster und das Predicate; CLI und Tests kennen keine rohen `log`-Aufrufe.

## Zustandsmaschine

- idle → running: Start-Ereignis (z. B. `BackupDispatching`).
- running → success: Abschluss-Ereignis mit Erfolgskennung.
- running → failed: Fehler-Ereignis oder Start ohne Abschluss im Fenster ("hängt seit X").
- kein Zustand, nur Ableitung: bei jedem Lauf aus dem Ereignisstrom neu konstruiert; nichts wird persistiert.

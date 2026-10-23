# tmsentinel

Zustandsloses, FDA-freies Time-Machine-Monitoring-CLI in Go. Es beantwortet:
Läuft mein Time-Machine-Backup wirklich — und wenn nicht, warum nicht?

Read-only, kein Daemon, keine GUI, keine State-Datei: Der Zustand „letzter
erfolgreicher Backup" wird bei jedem Lauf neu aus dem Unified Log abgeleitet.

Status: **Phase 1 — MVP: `check`, `destinations`** (Phase 0 abgeschlossen,
Gate 1 bestanden — siehe „Phase 0 Ergebnisse").

## Nutzung

```sh
go build -o tmsentinel .
./tmsentinel check [--json] [--threshold HOURS]   # Frische prüfen, Exit-Code für Scheduler
./tmsentinel destinations [--json]                # Ziele, Typ, Erreichbarkeit
./tmsentinel export FILE                           # Rohdaten-Export (Korpus/Fixtures)
./tmsentinel version
```

`check` leitet den Zustand frisch aus dem Unified Log ab (read-only,
kein Daemon, keine State-Datei) und prüft das Ziel per
`tmutil destinationinfo` (FDA-frei). Die zugrunde liegende Abfrage:

```
/usr/bin/log show --style ndjson --info --last 7d \
  --predicate 'subsystem == "com.apple.TimeMachine"'
```

(`--info` ist Pflicht: backupd loggt seine Lifecycle-Signale auf
Info-Stufe, siehe „Phase 0 Ergebnisse".)

### Exit-Codes (Vertrag)

| Code | Bedeutung |
| --- | --- |
| 0 | Letzter Backup erfolgreich und frisch; laufender Backup gilt als gesund (0 mit Hinweis) |
| 1 | Interner Fehler (Subprozess, Flags, Schreibfehler) |
| 2 | Letzter Backup älter als Schwelle — oder kein erfolgreicher Backup im Fenster |
| 3 | Kein Ziel konfiguriert |
| 4 | Ziel konfiguriert, aber nicht erreichbar |
| 5 | Reserviert: laufender Backup, nur für `--json`-Verbraucher (Feld `running: true`); `check` selbst beendet sich mit 0 + Hinweis |

Codes 0–5 sind stabil. Neue Zustände bekommen neue Codes ab 6, nur mit
Major-Version.

### Flags und JSON

- `check --threshold` (Stunden, Default 48), `check --json`,
  `destinations --json`
- `--json`-Schema von `check` (additiv, nie umbenennen): `state`,
  `last_success_age_h`, `threshold_h`, `destinations[]` (Name, Kind,
  Mount Point, reachable), `diagnosis{}`, `running`, `config_loaded`
- Keine Konfigurationsdatei nötig; `config_loaded: false` bis
  Konfiguration in Phase 2 dazukommt

### Export-Format

`tmsentinel export FILE` schreibt Header-Zeilen beginnend mit `#`
(captured_at, command), danach die unveränderten NDJSON-Datensätze.
Der Parser überspringt `#`-Zeilen und den `log show`-Footer
(`{"count":N,"finished":1}`). Ungeparste Zeilen werden mit Rohinhalt
auf stderr geloggt, gefolgt von einer Zusammenfassung
(`n parsed / n unparsed records`) — nichts wird still verworfen.

## Installation

### Homebrew (Tap)

```sh
brew install breichr/tmsentinel
```

(Tap: `github.com/breichr/homebrew-tmsentinel`; Formel-Quelle liegt als
Template in `contrib/homebrew/`.)

### Direkt-Download

Release-Archiv für `darwin-arm64` bzw. `darwin-amd64` von der
[GitHub-Releases-Seite](https://github.com/breichr/tmsentinel/releases)
entpacken, Binary nach `~/.local/bin` (o. ä.) legen und ausführbar machen.

### Aus dem Source

```sh
go build -o ~/.local/bin/tmsentinel .
```

Nur Standardbibliothek, keine externen Abhängigkeiten; nur macOS
(`statfs`-Erreichbarkeit und `log`/`tmutil` sind darwin-spezifisch).

## Nutzungsbeispiele

```sh
# Für launchd/cron/healthchecks.io: nur der Exit-Code entscheidet
tmsentinel check

# Warum ist nichts frisch? Ziele ansehen
tmsentinel destinations

# Eigene Schwelle
tmsentinel check --threshold 24

# Rohdaten für Fehlerberichte und Test-Fixtures
tmsentinel export report.ndjson
```

## Phase 0 Ergebnisse

Verifiziert am 2026-10-23 auf dem Ziel-Mac (macOS, arm64), ohne Full Disk
Access. Wichtig für die Interpretation: Auf diesem System ist das
Backup-Ziel seit Beginn des verfügbaren Logfensters **nicht erreichbar**
(laut `destinationinfo` ein lokales Ziel „TimeMachine", Volume aktuell
nicht gemountet; zusätzlich pingt der TimeMachine-Menüprozess ein
SMB-Ziel `NASBOX`, das per Bonjour nicht auflösbar ist). Im 7-Tage-Fenster
**fand kein einziger Backuplauf statt**.

### tmutil-Verifikation ohne Full Disk Access

**`tmutil destinationinfo` — funktioniert ohne FDA (exit 0):**

```
====================================================
Name          : TimeMachine
Kind          : Local
ID            : 8F11C3F6-D15E-42D4-91B2-DD75A321E475
```

**`tmutil status` — funktioniert ohne FDA (exit 0):**

```
Backup session status:
{
    ClientID = "com.apple.backupd";
    Percent = "-1";
    Running = 0;
}
```

**`tmutil latestbackup` — FDA-Bindung bestätigt (mit gemountetem Ziel):**

```
tmutil: latestbackup requires Full Disk Access privileges.
To allow this operation, select Full Disk Access in the Privacy
tab of the Security and Privacy preference pane, and add Terminal
to the list of applications which are allowed Full Disk Access.
```

(exit 80). Beim ersten Verifikationslauf (Ziel nicht gemountet) schlug
der Befehl erwartungsgemäß **nicht** am FDA fehl, sondern vorher am
Mounten des Ziels (`Code=18 "Failed to mount destination."`, bemerkenswerterweise
exit 0) — die FDA-geschützte Operation wurde nie erreicht. Mit gemountetem
Ziel tritt der FDA-Fehler exakt wie erwartet ein: **die FDA-Bindung ist
damit belegt.** Auch ein schlichtes `ls /Volumes/TimeMachine/` scheitert
ohne FDA mit „Operation not permitted" — das Ziel selbst bleibt FDA-geschützt,
der Unified Log nicht.

### Prototyp-Lauf gegen die realen Daten (7 Tage)

Zwei Läufe, zwei Systemzustände:

**Lauf 1 — Ziel nicht erreichbar (14:29 Uhr):**

```
tmsentinel: 5572 parsed / 0 unparsed records (1 footer lines)
Time Machine status (unified log, last 7d)
  last successful backup : none in window
  backups detected       : 0
  last backup state      : no backup session detected
  threshold              : 48.0 h
result: no successful backup in window (exit 2)
```

Gegenprobe: `tmutil status` meldete `Running = 0`, das Ziel-Volume war
nicht gemountet, das NAS per Bonjour nicht erreichbar — keine Quelle
zeigte einen Backup. Konsistent.

**Lauf 2 — nach anstecken der Backup-SSD, ein vollständiger Lauf (15:0x Uhr):**

```
tmsentinel: 13038 parsed / 0 unparsed records (1 footer lines)
Time Machine status (unified log, last 7d)
  last successful backup : 2026-10-23 14:37:01 CEST (0.3 h ago)
  backups detected       : 1
  last backup state      : success
  threshold              : 48.0 h
result: fresh (exit 0)
```

Gegenprobe Realität: Time Machine startete beim Anstecken automatisch
(`tmutil status`: `Running = 1` ab 14:33:49, Phase PreparingSourceVolumes)
und meldete danach `Running = 0`. Der Unified Log zeigt den vollständigen
Lauf: Dispatch 14:33:47, „Backup succeeded — Elapsed time: About 4 minutes"
14:37:01 — gemeldetes Alter (0,3 h) und Zeitstempel passen zur Realität.
Der vorherige Backup auf dem Ziel datierte vom 14.10. (Referenz-Mount
`2026-10-14-170330.backup`), das Fenster dazwischen war backupfrei, wie
Lauf 1 meldete.

### Wichtiger technischer Befund: Info-Stufe ist Pflicht

Der erste Prototyp lief ohne `--info` und sah **kein einziges**
Lifecycle-Ereignis. Ursache: `backupd` loggt sämtliche Lebenszyklus-Signale
(`BackupDispatching`, `BackupDestination`, `TMSession`, …) auf **Info**-Stufe,
die `log show` standardmäßig nicht zurückgibt. Die finale Abfrage lautet
daher:

```
/usr/bin/log show --style ndjson --info --last 7d \
  --predicate 'subsystem == "com.apple.TimeMachine"'
```

Zweiter Befund: `fs_snapshot_list failed: Operation not supported` ist
**kein** Fehlersignal — es feuert während eines gesunden, erfolgreichen
Backups (3× bei 14:33:51, Abschluss success) ebenso wie in der idle-Wartung
(62× über die backupfreie Woche). Der Klassifikator behandelt es
entsprechend als Rauschen; ebenso `Starting backup pass N` (innere
Durchgänge eines laufenden Backups, Kategorie FileProtection).

Nebenbefund: Der Unified-Log-Speicher dieses Systems reicht nur bis
2026-10-19 13:24 zurück (eine 30-Tage-Abfrage liefert praktisch dieselben
Zeilen wie die 7-Tage-Abfrage). Für die Korpus-Aufnahme heißt das:
Aufnahmen sind kurzlebig, `--export` regelmäßig ausführen.

## Gate-1-Bericht

**Gate-1-Frage: Liefert der Unified Log zuverlässig Start und Abschluss
jedes Backups ohne FDA?**

Antwort: **Ja — belegt an einem realen, vollständigen Backuplauf am
2026-10-23 auf diesem Mac, vollständig ohne FDA.** Start und Abschluss
erscheinen beide in der Kategorie `BackupDispatching` des Subsystems
`com.apple.TimeMachine` (Stufe Info) mit klaren, maschinenlesbaren
Meldungen.

Je Backup im 7-Tage-Fenster:

| Backup | Start-Ereignis | Abschluss-Ereignis |
| --- | --- | --- |
| 2026-10-23, 14:33:47 → 14:37:01 (lokal, APFS-SSD) | ja — `BackupDispatching`: „Backup requested to next destination in rotation" | ja — `BackupDispatching`: „Backup succeeded — Elapsed time: About 4 minutes" |

Vorherige Backups (zuletzt 14.10.) liegen außerhalb des Logfensters
(Speicher retenziert nur ~4 Tage) und sind daher nicht prüfbar — keine
Aussage über sie, weder positiv noch negativ.

Belegt außerdem:

- `log show --style ndjson --info` auf Subsystem `com.apple.TimeMachine`
  funktioniert **ohne FDA** aus der Kommandozeile; im 7-Tage-Fenster
  13.038 Datensätze lesbar, alle Felder öffentlich (kein `<private>`).
- Das **Fehlen** von Backups ist ebenso beobachtbar: Lauf 1 meldete
  korrekt „kein Backup im Fenster", während das Ziel nicht erreichbar war
  — der Anwendungsfall des Werkzeugs, FDA-frei.
- Die FDA-Grenze liegt exakt dort, wo das Projekt sie vermutet hat:
  `tmutil latestbackup` (exit 80) und selbst `ls` auf dem Ziel scheitern
  ohne FDA; der Unified Log nicht.

Offene Fragen (dokumentiert, nicht spekulativ im Code gelöst):

1. Auf diesem Mac ist **derzeit kein Time-Machine-Netzwerkziel**
   konfiguriert (lokal, APFS-SSD). Die SMB-Variante von `BackupDispatching`
   ist hier daher **nicht anwendbar** — offene Frage für später oder
   Fremdsysteme, sobald ein Netzwerkziel existiert.
2. Wie sieht das **Fehlschlag**-Abschluss-Ereignis dieser macOS-Version
   aus (erwartet: Kategorie `BackupDispatching`, Meldung mit „failed")?
   Klassifikation aktuell nur mit klassischen Mustern abgedeckt, auf
   diesem System unverifiziert.
3. Ein realer **hängender** Backup (Start ohne Abschluss) lag im Fenster
   nicht vor; die Zustandsmaschine behandelt ihn (hung nach 6 h Gnadenfrist),
   aber ohne Fixture bleibt er ungetestet gegen Realität.

Gate 1: **bestanden** (lokal bewiesen; SMB-Variante mangels Netzwerkziel
auf diesem System nicht prüfbar, als offene Randbedingung dokumentiert).

### Musterkatalog des Prototyps mit Verifizierungsstatus

| Muster | Klassifikation | Status |
| --- | --- | --- |
| Kategorie `BackupDispatching` + „Backup requested to next destination in rotation" | start | **belegt** (realer Lauf 2026-10-23) |
| Kategorie `BackupDispatching` + „Backup succeeded" | success | **belegt** (realer Lauf 2026-10-23) |
| `Starting backup pass N` (FileProtection) | none (innerer Durchgang) | **belegt** — bewusst kein Sitzungs-Start |
| `fs_snapshot_list failed` | none (Rauschen) | **belegt** — feuert auch in gesunden Läufen |
| `Failed to read capabilities`, `Bonjour resolution not possible`, XPC-Rauschen | none (Rauschen) | **belegt** |
| `Starting (automatic\|manual) backup` | start | klassisch dokumentiert, auf diesem System **unverifiziert** |
| `Completed backup`, `Backup completed`, `Backup finished` | success | klassisch dokumentiert, auf diesem System **unverifiziert** |
| `Backup failed`, `failed to complete backup`, `Backup aborted`, `Failed to mount (backup )?destination` | failed | klassisch dokumentiert, auf diesem System **unverifiziert** |

### Korpus-Start

Zwei anonymisierte Real-Exporte als Golden-File-Tests:

- `testdata/phase0-2026-10-23-anonymized.ndjson` — backupfreies Fenster
  (Default-Stufe, 5570 Datensätze): Zustand „kein Backup", Exit 2.
- `testdata/phase0-2026-10-23-run.ndjson` — vollständiger realer Lauf
  (Info-Stufe, 292 Datensätze): genau eine Sitzung, Start 14:33:47,
  Erfolg 14:37:01, Exit 0 — die Gate-1-Evidenz.

Weiterführung: regelmäßig `--export` ausführen, bis der Korpus auch einen
SMB-Lauf (Erfolg und Abbruch) und einen hängenden Backup enthält (Roadmap).

## Definition of done (Phase 0)

- `go build ./...`, `go vet ./...`, `go test ./...` bestehen — Unit-Tests
  für Parser, Klassifikation, Zustandsmaschine, Schwellenlogik und Export
  mit Beispiel-NDJSON-Zeilen, inkl. Zeilen mit fehlenden Feldern
  (kein Timestamp, fehlende Felder, Müllzeilen, Footer, Kommentarzeilen).
- Read-only: das Programm startet/stoppt keine Backups und verändert nichts
  am System.
- Keine Phase-1-Arbeit (`check`/`destinations`-Subcommands).

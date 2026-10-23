# tmsentinel

Zustandsloses, FDA-freies Time-Machine-Monitoring-CLI in Go. Es beantwortet:
Läuft mein Time-Machine-Backup wirklich — und wenn nicht, warum nicht?

Read-only, kein Daemon, keine GUI, keine State-Datei: Der Zustand „letzter
erfolgreicher Backup" wird bei jedem Lauf neu aus dem Unified Log abgeleitet.

Status: **Phase 0 — Beweis des FDA-freien Kerns** (Prototyp, siehe
„Phase 0 Ergebnisse").

## Nutzung (Phase 0 Prototyp)

```sh
go build -o tmsentinel .
./tmsentinel [--threshold HOURS] [--export FILE]
```

Der Prototyp führt als Subprozess aus (read-only):

```
/usr/bin/log show --style ndjson --last 7d \
  --predicate 'subsystem == "com.apple.TimeMachine"'
```

Ausgabe: Zeitstempel und Alter des letzten erfolgreichen Backups, Anzahl
erkannter Backups im 7-Tage-Fenster, Zustand des letzten Backups
(success / failed / running / hung — Start ohne Abschluss), Schwelle.

Ungeparste Zeilen werden mit Rohinhalt auf stderr geloggt, gefolgt von einer
Zusammenfassung (`n parsed / n unparsed records`). Nichts wird still
verworfen.

### Exit-Codes (Vertrag)

| Code | Bedeutung |
| --- | --- |
| 0 | Letzter Backup erfolgreich und jünger als die Schwelle |
| 1 | Interner Fehler (Subprozess, Flags, Export-Schreibfehler) |
| 2 | Letzter Backup älter als die Schwelle — oder kein erfolgreicher Backup im Fenster |

### Flags

| Flag | Default | Bedeutung |
| --- | --- | --- |
| `--threshold` | 48 | Frische-Schwelle in Stunden |
| `--export` | – | Schreibt die rohe `log show`-Ausgabe mit Metadaten-Header in eine Datei (Korpus-Start; wird später `tmsentinel export`) |

Export-Format: Header-Zeilen beginnend mit `#` (captured_at, command),
danach die unveränderten NDJSON-Datensätze. Der Parser überspringt
`#`-Zeilen und den `log show`-Footer (`{"count":N,"finished":1}`).

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

**`tmutil latestbackup` — erwarteter FDA-Fehler trat NICHT ein:**

```
Failed to mount backup destination, error: Error Domain=com.apple.backupd.ErrorDomain Code=18 "Failed to mount destination." UserInfo={NSLocalizedDescription=Failed to mount destination.}
```

Befund: Der erwartete FDA-Fehler („Operation not permitted") erscheint in
diesem Systemzustand nicht, weil `latestbackup` bereits am Mounten des
Ziels scheitert — die FDA-geschützte Operation wird nie erreicht. Bemerkenswert:
der Befehl endet trotz Fehler mit **exit 0**. Die FDA-Bindung ist damit auf
diesem System **nicht belegt**; offene Frage, mit gemountetem Ziel neu zu
verifizieren (siehe Gate-1-Bericht).

### Prototyp-Lauf gegen die realen Daten (7 Tage)

```
$ ./tmsentinel --export corpus-2026-10-23.ndjson
tmsentinel: 5572 parsed / 0 unparsed records (1 footer lines)
Time Machine status (unified log, last 7d)
  last successful backup : none in window
  backups detected       : 0
  last backup state      : no backup session detected (62 failure signals, last: 2026-10-23 14:06:12 CEST)
  threshold              : 48.0 h
  export                 : corpus-2026-10-23.ndjson
result: no successful backup in window (exit 2)
```

Gegenprobe Realität: `tmutil status` meldet `Running = 0`, das Ziel-Volume
ist nicht gemountet, das NAS ist per Bonjour nicht erreichbar — es gibt
also auch nach außerlogischer Quelle **keinen** aktuellen Backup. Die
Meldung „kein erfolgreicher Backup im Fenster, 62 Fehlersignale" deckt
sich mit der Realität. Ein Alter konnte nicht gegengeprüft werden, weil
kein erfolgreicher Backup existiert; die Zustandsaussage ist konsistent
über alle Quellen.

Nebenbefund: Der Unified-Log-Speicher dieses Systems reicht nur bis
2026-10-19 13:24 zurück (eine 30-Tage-Abfrage liefert praktisch dieselben
Zeilen wie die 7-Tage-Abfrage). Für die Korpus-Aufnahme heißt das:
Aufnahmen sind kurzlebig, `--export` regelmäßig ausführen.

## Gate-1-Bericht

**Gate-1-Frage: Liefert der Unified Log zuverlässig Start und Abschluss
jedes Backups ohne FDA?**

Antwort: **Auf diesem System derzeit nicht belegbar — es gab im
7-Tage-Fenster keinen einzigen Backuplauf**, also auch kein Start-/Abschluss-Paar
zum Gegenüberstellen.

Je Backup im 7-Tage-Fenster:

| Backup | Start-Ereignis | Abschluss-Ereignis |
| --- | --- | --- |
| (keine Backups im Fenster — 0 Sitzungen erkannt) | – | – |

Was belegt ist:

- `log show --style ndjson` auf Subsystem `com.apple.TimeMachine`
  funktioniert **ohne FDA** aus der Kommandozeile: 5570 Datensätze in 7
  Tagen lesbar, alle Felder öffentlich (kein `<private>` in dieser
  Aufnahme).
- Das **Fehlen** von Backups ist sauber beobachtbar: 62
  `fs_snapshot_list failed`-Signale, 2033 „Failed to read capabilities"-
  Meldungen (NAS-Volume), 1511 Bonjour-Auflösungsfehler — das Werkzeug
  hätte also exakt das getan, wofür es gebaut ist: melden, dass das
  Backup nicht läuft.

Was nicht belegt ist:

- Ob Start- und Abschluss-Ereignisse eines **real erfolgreich
  durchgelaufenen** Backups im Subsystem `com.apple.TimeMachine`
  erscheinen und mit welchen exacten Meldetexten auf dieser macOS-Version.
  Die Start-/Erfolgsmuster des Prototyps sind dokumentiert-klassisch
  (siehe Tabelle unten), aber ohne reales Fixture auf diesem System.

Konsequenz und nächste Schritte (keine Spekulation im Code — ungeklärtes
bleibt offene Frage):

1. Die Bestätigung von Gate 1 erfordert mindestens einen real
   durchgelaufenen Backup. Der Prototyp ist dafür fertig: `--export`
   schneidet rohe Ausgaben mit, die Klassifikationsmuster werden dann
   gegen den Korpus verifiziert.
2. Die Muster für start/success sind bis dahin als **unverifiziert** zu
   behandeln; der Korpuslauf der nächsten zwei Wochen (Roadmap Phase 0)
   liefert die Evidenz.
3. Offene Frage: FDA-Bindung von `tmutil latestbackup` mit gemountetem
   Ziel klären (aktuell scheitert der Befehl vorher am Mount, exit 0).

### Musterkatalog des Prototyps mit Verifizierungsstatus

| Muster | Klassifikation | Status |
| --- | --- | --- |
| `Starting (automatic\|manual) backup`, `Starting backup`, eventType `BackupDispatching` | start | klassisch dokumentiert, auf diesem System **unverifiziert** |
| `Completed backup`, `Backup completed`, `Backup finished` | success | klassisch dokumentiert, auf diesem System **unverifiziert** |
| `fs_snapshot_list failed` | failed | **belegt** (62× im Korpus-Sample) |
| `Failed to mount (backup )?destination` | failed | plausibel, im Korpus nur außerhalb (tmutil) beobachtet |
| `Backup failed`, `failed to complete backup`, `Backup aborted` | failed | klassisch dokumentiert, **unverifiziert** |
| `Failed to read capabilities`, `Bonjour resolution not possible`, XPC-Rauschen | none (Rauschen) | **belegt** (bewusst nicht als Fehlersignal klassifiziert) |

### Korpus-Start

`testdata/phase0-2026-10-23-anonymized.ndjson` enthält den anonymisierten
Real-Export (5570 Datensätze, 0 Backups, 62 Snapshot-Fehler) als
Golden-File-Test. Weiterführung: regelmäßig `--export` ausführen, bis der
Korpus mindestens einen vollständigen Backuplauf (start → success) und
einen SMB-Fehler enthält (Roadmap).

## Definition of done (Phase 0)

- `go build ./...`, `go vet ./...`, `go test ./...` bestehen — Unit-Tests
  für Parser, Klassifikation, Zustandsmaschine, Schwellenlogik und Export
  mit Beispiel-NDJSON-Zeilen, inkl. Zeilen mit fehlenden Feldern
  (kein Timestamp, fehlende Felder, Müllzeilen, Footer, Kommentarzeilen).
- Read-only: das Programm startet/stoppt keine Backups und verändert nichts
  am System.
- Keine Phase-1-Arbeit (`check`/`destinations`-Subcommands).

---
name: |
  tmsentinel-implementation
description: |
  Use this skill when implementing, testing, or extending the tmsentinel project — a stateless, FDA-free Time Machine monitoring CLI in Go (read-only; answers "is my backup really running, and if not, why?"). Load it before writing any tmsentinel code, reviewing a PR, or answering design questions about Unified-Log parsing, exit codes, why-fingerprints, fixtures, launchd integration, or release packaging.
---

# tmsentinel implementation

Projektstatus: Spezifikationsphase, Repository existiert noch nicht. Bis ein Repository mit eigenem `AGENTS.md` existiert, sind dieses Skill und seine Support-Dateien die Source of Truth. Danach gilt `AGENTS.md` im Repository-Root zuerst.

Arbeitsname: tmsentinel (tmwatch ist vergeben). Sprache im Code und in Commits: Englisch. Sprache in Nutzer-Antworten: wie im Gespräch etabliert.

## Load order

1. `01-spezifikation.md` — CLI-Oberfläche, Exit-Codes, Konfiguration. Exit-Codes sind ein Kompatibilitätsvertrag, niemals ändern.
2. `02-architektur.md` — Modulgrenzen, Datenquellen, Subprozess-Entscheidungen.
3. `03-teststrategie.md` — vor jeder Test- oder Fixture-Arbeit.
4. `04-roadmap-gates.md` — ordnet jede Aufgabe einer Phase zu; Gates und Abbruchkriterien sind verbindlich.
5. `05-agents-template.md` — nach `go mod init` AGENTS.md im Repository-Root aus diesem Template anlegen; ab dann ist AGENTS.md die Source of Truth.

## Invarianten, die Feature-Wünsche überlagern

- Zustandslos. Keine Datenbank, keine State-Datei, kein Daemon. Der Zustand "letzter erfolgreicher Backup" wird bei jedem Lauf aus dem Unified Log abgeleitet. Ein zweiter Lauf ändert nichts.
- Read-only. Das Tool startet, stoppt und konfiguriert keine Backups. Schreibender Zugriff: keiner, auch nicht hinter einem Flag.
- FDA-frei. Kein Full Disk Access erforderlich. `tmutil latestbackup`/`listbackups` nur hinter optionalem FDA-Flag.
- Exit-Codes 0–5 sind stabil und dokumentiert. Änderungen nur additiv (neue Codes ab 6) und mit Major-Version.
- Keine GUI, kein TUI, kein eigenes Backup-Scheduling, keine Flotten-Features. Jedes davon ist ein besetztes Nachbarsegment.
- Log-Parsing über `log show --style ndjson` als Subprozess, nicht OSLogStore. Parsing robust gegen fehlende Felder; ungeparste Datensätze mitloggen, nie verwerfen und raten.
- Fingerabdruck-Diagnosen nur mit belegtem Test-Fixture. Unverstandene Muster bleiben unveröffentlicht — eine falsche Diagnose ist schlimmer als keine.

## Definition of done für einen Change

- `go build ./...`, `go vet ./...`, `go test ./...` bestehen.
- Neue Parsing-Logik hat mindestens einen Fixture-Test, inklusive des Fallbacks bei fehlenden Feldern.
- `tmsentinel check` funktioniert nach jedem Change weiterhin ohne Konfigurationsdatei.
- Bei nutzerseitig sichtbarem Verhalten: README aktualisieren.
- Bei neuen Ereignis-Mustern: Fixture in `testdata/` und Eintrag im Fingerabdruck-Katalog, sonst kein Merge.

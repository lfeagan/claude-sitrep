# The colour tool

Design note. Not built yet.

## What it is

`sitrep colors` is a full-screen tool for choosing the dashboard's colours. It draws a preview of every place a colour is used, and a list of the settings with a picker for each. A change shows in the preview at once. `w` writes the result to `sitrep.conf`. The same screen opens from `sitrep dash` with `C`, over the real sessions.

It is one Bubble Tea model (`internal/tui/colors.go`) that uses the dashboard's own `themeFor` and `render`, so the preview cannot drift from the dashboard. The preview data is the `--demo` snapshot, or the live snapshot when opened from `dash`.

## The screen

```
 Colours        theme: rainbow   ▸ dark terminal  (t: light)
─────────────────────────────────────────────────────────────
 PREVIEW (the dashboard's Cards, Table and Wall, as they draw)
  ...rows: needs you, faded, recent activity, snoozed, working, ended...
  ...tab bar, status bar, an error line, dim text...
─────────────────────────────────────────────────────────────
 ▶ theme      rainbow
   accent     #7ddc8a   (light #2e7d32)
   dim        #8fb0e0   (light #4a6fa5)
   hot        #ffb870   ...
   attn       #ffe566   ...
   error      #ff6b6b   ...
   snoozed    #64b5f6   ...
 j/k move  enter pick  r reset this one  t light/dark  w write  q quit
```

- The preview is drawn for one kind of terminal at a time, `dark` or `light`, and `t` switches. The tool asks the terminal which it is, as the dashboard does, and starts on that.
- A colour has a light and a dark value. The picker edits the one on screen. `L` copies it to the other side, which is the common case of "one colour for both".
- Rows that a setting changes are marked in the list with `*` when it differs from the theme's own value.

## The picker

`enter` on a colour opens a picker under the list:

- a hex field (`#rrggbb`, typed, with `ctrl+u` and `ctrl+w` as in the other prompts);
- the 256-colour grid, drawn as small blocks, moved with `hjkl` (`H`/`L` move by ten); `tab` switches between the hex field and the grid;
- `enter` accepts, `esc` cancels and the preview goes back.

While the picker is open the preview already shows the candidate. For `attn` and `hot`, which are backgrounds, the line also shows the text colour sitrep chooses for it and the contrast ratio (WCAG, as a number and `ok` or `low`, below 4.5). `accent` shows the same for the status bar.

The theme row cycles the five themes with `h`/`l`. Choosing a theme clears nothing the user set: the colours stay as overrides on top of it.

## What it writes

Two lines of `sitrep.conf`, and no others:

- `SITREP_THEME=<name>`, written only when the theme is not `default`; removed when it is, or set back to default.
- `SITREP_COLORS='name=value,...'`, only for colours that differ from the theme's own value. A colour that equals the theme's is not written. When nothing differs the line is removed.

Everything else in the file is the user's, and stays as it is, byte for byte: other lines, comments, the examples block, a line's position. A line this tool replaces is replaced in place, so the file does not reorder. When there is no such line, the new one goes just above the examples block, or at the end when there is none. A commented example line (`# SITREP_COLORS=...`) is never touched.

### The write, step by step (`w`)

1. Read the colour settings the file holds, and show the old and the new `SITREP_THEME` and `SITREP_COLORS` lines in a question: `write these two lines to ~/.claude/sitrep/sitrep.conf?  y/n`. The question names the file the path resolves to when `sitrep.conf` is a link.
2. On `y`: resolve the path (`filepath.EvalSymlinks`), so a dotfiles link is written through and stays a link. Refuse a path that is not a regular file after resolving, and a file larger than 64 KB, as `PutExamples` does.
3. Back the file up once before the first change of the session: `settings.Backup` (owner-only, a new name each time, never over an earlier backup), next to the file it resolved to. The status line says where.
4. Write by a unique temporary file in the folder of the resolved file, with the mode of the file it replaces (`0600` for a new one), published by rename. If the temporary file cannot be made there (a read-only dotfiles folder), nothing is changed and the message says so.
5. Read the file back and check that every line but the two is the same as before. If not, restore from the backup and say so. (This is the check `sitrep setup` makes for `settings.json`.)
6. Say `written; sitrep dash picks it up within a few seconds`.

A file that holds an environment override is still written, and the tool says that the environment wins: `SITREP_THEME` and `SITREP_COLORS` set in the shell beat `sitrep.conf` (conf.Get reads the environment first), so a preview that does not match the dashboard is explained, not mysterious. The tool starts with a one-line warning when either is set.

A file with a setting that is a link, a pipe or not regular is refused for writing as the other state files are; reading follows a link, as `sitrep.conf` always has (`state.ReadFileFollow`).

## Live reload in `dash`

Today the dashboard reads the theme settings with each snapshot (`Settings.Theme`, `Settings.Colors` are in the snapshot), so a written change appears at the next refresh, within the interval (3 s). The tool needs no signal. Nothing to build here, except a test that proves it.

## Where it lives

- `internal/conf`: `PutSettings(map[string]string, remove []string)` replaces or adds `NAME='value'` lines, with the steps above, returning the backup path. The `Quote` function is used for values. Tests with a link, a read-only folder, a missing file, a file with the example block, CRLF, and a line twice.
- `internal/tui/colors.go`: the model, the picker and the preview. `internal/tui/colors_test.go` and golden screens.
- `internal/cli/cli_colors.go`: `sitrep colors [--demo] [--light|--dark]`. The command needs no sessions. `--demo` is the default, and the command always uses made-up sessions (moved from `internal/cli/demo.go` to be shared with `dash --demo`). `dash` gets `C`, which opens the same model over the live snapshot.
- `docs/SETTINGS.md`, `docs/DASHBOARD.md`, `README.md`, `sitrep help colors`, the completion.

## Keys

`j`/`k` move, `enter` opens the picker, `r` resets this setting to the theme's value, `R` resets all (asks), `t` switches the preview between a dark and a light terminal, `L` copies the value to the other side, `w` writes (asks), `q` quits (asks when there is something unwritten), `?` the list. In the picker, `h`/`l` move by one colour step and `H`/`L` by ten, `tab` switches between the hex field and the grid, `enter` accepts and `esc` cancels.

## Not in this version

- No `hot` SGR editing (`SITREP_HIGHLIGHT_SGR`): it is an older setting that `hot` replaces.
- No mouse in the first version.
- No saving of named palettes. The user's dotfiles are the place for that.
- No change to `~/.claude/settings.json` or the status line colours: this is for the dashboard.

## Risks

- Writing to a user's dotfile is new. It is guarded by the question, the backup, the write through a link, the read-back check and the restore. The tests prove each of those, and one test breaks each guard to see it fail.
- A colour the terminal cannot show (a 256-colour number on a 16-colour terminal) is the user's to see in the preview, which is drawn by the same code as the dashboard.
- The preview is only as truthful as the terminal's own colour support; `NO_COLOR` is honoured and the tool says so rather than drawing nothing.

# The colour tool

Design note. Built as described, except that the preview starts on the Table tab and `tab` moves it to the Wall and the Cards.

## What it is

`sitrep colors` is a full-screen tool for choosing the dashboard's colours. It draws a preview of every place a colour is used, and a list of the settings with an editor for each. A change shows in the preview at once. `w` writes the result to `sitrep.conf`, and a running `sitrep dash` shows it at its next refresh.

It is one Bubble Tea model (`internal/tui/colors.go`). The preview is made by building the `SITREP_COLORS` string the tool would write and passing it, with the theme name, to the dashboard's own `themeFor` and `render`, so there is one path and the preview cannot drift from the dashboard. The preview data is the made-up `--demo` sessions. The command needs no sessions and no install.

The `C` key of `sitrep dash` opens the same model over the dashboard's own sessions, and `q` returns to it.

## The screen

```
 Colours        theme: rainbow   preview: dark terminal  (t: light)
──────────────────────────────────────────────────────────────────
 the Cards, Table and Wall as they draw, with a row for each of:
 needs you, faded, recent activity, snoozed, working, ended; the
 tab bar, the status bar, an error line, dim text
──────────────────────────────────────────────────────────────────
 ▶ theme      rainbow
   accent     #7ddc8a
   dim        #8fb0e0
   hot        #ffb870   *
   attn       #ffe566
   error      #ff6b6b
   snoozed    #64b5f6
 j/k move  enter edit  r reset  t light/dark  w write  q quit
```

- The preview is drawn for one kind of terminal at a time, `dark` or `light`. `t` switches. It starts on what the terminal says it is (the dashboard's `BackgroundColorMsg`), and on `dark` until the terminal answers.
- A `*` marks a change that is not written yet, and `set` marks a colour that the file holds; a colour with no mark is the theme's own. A colour is set as explicit state: a colour is set when it is in the file (or edited here), even when its value equals the theme's own. That matters, since any name in `SITREP_COLORS` also switches on the chosen text colour for the `hot` and `attn` backgrounds (`custom[name]` in `themeFor`), so `hot=238` is not the same as no `hot`. `r` clears one setting back to the theme's own, and that is the only way an entry leaves the line.
- Values are shown for the terminal kind on screen. A colour has a light and a dark value. Editing one side writes `light/dark`, with the other side being the value it has now. When the two sides end up equal, one value is written.
- Under the `mono` theme the colour rows are greyed with the line `mono has no colours; pick another theme`, because `themeFor` ignores `SITREP_COLORS` there. Under `NO_COLOR` the tool starts with `NO_COLOR is set: the preview shows no colour`, and the editing still works.
- When `SITREP_THEME` or `SITREP_COLORS` is set in the environment, the tool starts with a warning that the environment beats the file (`conf.Get` reads it first), so what it writes will not show until the variable is unset. The tool starts from the values the file holds (`FromFile`), never from the environment's, so it never writes the shell's values into the file.
- On a short screen the preview gives way first: the preview starts on the Table tab, which shows every kind of row in the fewest lines (`tab` switches it to the Wall and the Cards), and it takes what the list and the footer leave. Below 16 rows the tool says the screen is too small.

## The editor

`enter` on a colour opens a one-line field under the list. It takes `#rrggbb` or a terminal colour number from 0 to 255 (the two forms `parseColors` reads), with `ctrl+u` and `ctrl+w` as in the other prompts. The candidate shows in the preview while it is typed, and an entry that is not a colour is shown as it is and refused at `enter`. `up` and `down` step a number by one, so a colour can be found by walking. The footer shows the lightness, chroma and hue of the colour in the field (`L 65  C 0.04  h 210`); the first hue turn of a grey gives it only a little chroma, so it stays calm. For a colour in hex, `h`/`l` turn the hue, `j`/`k` change the lightness and `H`/`L` the chroma, in the OKLCH model (`internal/tui/oklch.go`): turning the hue keeps the lightness, which HSL does not. A chroma the screen cannot show is kept in the field's state and cut only when drawn, so turning the hue round and back returns the colour. The candidate shows in the preview while the field is open. `enter` accepts and `esc` cancels.

`enter` on the theme row opens a gallery of the themes (`ThemeNames`), one line each in its own colours: the bar, recent activity, a session that needs you, a snoozed one, dim text and an error. `j`/`k` move, `enter` chooses, `t` switches between a dark and a light terminal, `esc` goes back. `h` and `l` on the theme row still step through the themes without it. Choosing a theme keeps the user's colours as overrides on top of it.

Not in the first version: a grid of the 256 colours (stepping in OKLCH took its place), contrast ratios, a copy between the light and dark side, and a reset-all key.

## What it writes

Two lines of `sitrep.conf`, and no others:

- `SITREP_THEME=<name>`, written when the theme is not `default`. It is removed when the theme is `default`.
- `SITREP_COLORS=<value>`, the entries that are set, in the order `accent, dim, hot, attn, error, snoozed`. It is removed when none is set.

A line is ours when it starts with `SITREP_THEME=` or `SITREP_COLORS=` at the first column, the same test `FromFile` makes: a line with `export`, a leading space or a `#` is the user's and is left alone. The first such line is replaced where it stands. Any later line of the same name is removed, because the reader takes the first and the later ones are dead, and the tool says so. A name with no line goes just above the examples block, or at the end of the file when there is none. Every other byte stays as it was, and a file that uses `\r\n` gets the new lines with `\r\n`.

A value is checked as `Conf.Colors()` and `Conf.Theme()` check it (the allowed characters, at most 300 characters, a theme name of letters and `-`), and the write is refused when it would not pass, since the dashboard would ignore the whole line.

### The write (`w`)

1. A question names the file (the one a link points to, when `sitrep.conf` is a link) and shows the old and the new lines: `write these lines?  y/n`. When nothing differs it says `nothing to write`.
2. On `y`, the file is read again. When it is not what the screen was drawn from (the user edited it meanwhile), the tool starts again from the new content and asks again.
3. The bytes just read are saved to `sitrep.conf.bak.<timestamp>` in the state folder (owner-only, never over an earlier backup, `settings.Backup`), before every write that would replace different bytes. The state folder, not the dotfiles repo, so a repository is not filled with backups.
4. The new content is written as `settings.Write` writes `settings.json`: the path is resolved through links (a dotfiles link stays a link), a temporary file with a unique name is made in the folder of the resolved file, with the mode of the file it replaces (`0600` for a new one), and it is renamed over the file only when the file still holds the bytes read in step 2 (`ErrChanged` otherwise: nothing is changed and the tool says so). A folder that cannot be written to is an error, and nothing is changed.
5. The file is read back and its lines other than the two are compared with the ones before. When they differ, the tool says so and names the backup. It does not restore by itself, since that could overwrite an edit the user made in between.
6. `written; sitrep dash shows it at its next refresh`.

`PutHUD`, `DropHUD`, `PutExamples` and `DropExamples` use `Store.WriteFile`, which renames over the path and so replaces a linked `sitrep.conf` with a plain file. They move to the same link-following writer, so setup stops breaking a dotfiles link too.

### Read-only

`SITREP_TUI_READONLY` (and `--read-only`) turn `w` off in the tool as they turn off every key that changes state in `dash`. The tool says so in the footer.

## Where it lives

- `internal/conf`: `Put(changes map[string]*string, ...)` replaces, adds or removes named lines by the rules above, and `writeThrough` is the link-following writer that the other four writers use. Tests: a link, a dangling link, a read-only folder, a missing file, the examples block, CRLF, a name twice, a quoted or `export`ed line, a commented example, a value that is too long or has a bad character, a file changed between read and write, a file of 64 KB and more. A test breaks each guard (the changed-file check, the link resolution, the first-line rule) to see it fail.
- `internal/tui/colors.go`: the model, the editor and the preview, with golden screens at three sizes and tests for the keys.
- `internal/cli/cli_colors.go`: `sitrep colors [--light|--dark] [--read-only]`. `internal/cli/demo.go`'s snapshot is shared with `dash --demo`.
- `AGENTS.md`: invariant 2 allows a temporary file next to the resolved file for `sitrep.conf` and `settings.json`. A new invariant says that the colour tool changes two named lines of `sitrep.conf` and nothing else, and recurses over nothing.
- `docs/SETTINGS.md`, `docs/DASHBOARD.md`, `README.md`, `sitrep help colors`, the completion.

## Live reload

Every snapshot builds a fresh `Conf` and reads `sitrep.conf` again, and the dashboard builds its theme from the latest snapshot on every frame. A written change shows within one refresh. A test proves it. A `dash` started with `SITREP_THEME` or `SITREP_COLORS` in its environment never sees the change, and `dash --demo` reads no file.

## Keys

`j`/`k` move, `tab` changes the tab the preview draws, `enter` edits, `r` resets this one to the theme's own, `t` switches the preview between a dark and a light terminal, `w` writes (asks), `q` quits (asks when something is unwritten), `h`/`l` on the theme row change the theme.

## Risks

- Writing to a user's dotfile is new. The question, the backup, the write through a link, the compare-before-rename and the read-back check guard it.
- A colour the terminal cannot show is the user's to see in the preview, which is drawn by the dashboard's own code.

## Reviewing a theme by eye, and for colour blindness

`go test ./internal/tui -run TestWriteThemeShots` writes the Table and Wall frames of each theme, with their colour codes, to the folder in `SITREP_SHOTS_DIR` (`SITREP_SHOTS_THEMES=winter,fall` picks themes, and `SITREP_SHOTS_COLORS='attn=#ffb8dc/#6a2a58'` tries a palette on top of a theme without editing it). `python3 docs/theme-shots/ansi2png.py <ans folder> <png folder>` turns them into sheets, one per theme and kind of terminal, that stack the frames as seen with normal sight and then with protanopia, deuteranopia and tritanopia (Machado 2009, full severity). It needs Python 3 and `rsvg-convert`. `CVD_ONLY=normal` makes the normal view only. The sheets are what a reviewer looks at.

The rule that came out of the first review: what needs you must differ from recent activity in lightness, not in hue alone, because red-green and blue-yellow colour blindness take the hue away. On a dark terminal the needs-you background is at least 1.6 times as light as the recent-activity one, and on a light terminal at most 0.8 times (`TestNeedsYouDiffersFromRecentActivityInLightness`, for every theme but `mono`; the default theme's dark recent-activity colour is the documented `238`, so its needs-you colour is lighter than that). Still open: in some themes the error block and the accent bar turn the same yellowish colour under red-green colour blindness; the text in them still tells them apart.

`make screenshots` redraws the pictures of the README (`docs/img`) from the made-up demo sessions: `TestWriteReadmeShots` in `internal/cli` writes the frames (`SITREP_README_DIR`), and `docs/theme-shots/readme_shots.py` draws them in windows and rewrites each as a palette PNG, which is a fraction of the size. Regenerate them only when the look changes, since every new version of a picture stays in the history.

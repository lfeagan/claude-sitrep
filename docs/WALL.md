# The Wall tab: many sessions at once

Status: built (0.19.0).

## Claim

Watch gives each session a block of five to eleven rows, so a 24-row pane shows two sessions, and Table gives each one row but shows almost nothing about it. A user who runs twenty sessions (the heavy-user persona, and the user's own idea of a "super-industrial" view) needs a third view that shows many sessions at once, with the state of each, and lets the eye find the ones that need them. A new tab, **Wall**, lays the sessions out as tiles in as many columns as the width allows.

## Rule

1. Every tile has the same height, whether or not its session has a queue. A grid only lines up if its cells are as high as one another, and `j` and `k` mean "the tile above and below" only then. A tile whose content is short leaves rows blank. A tile never changes its height when its session changes, so tiles do not jump on a refresh.
2. The number of columns is the width divided by 30 (at least one), and the tiles share the width. The tile rows scroll as a whole, with `+N above` and `+N more` lines that count tiles.
3. The order is the one of the Table tab sorted by state: sessions that need the user first, then the order of the other tabs. Selection moves through the tiles in that order, left to right and then down.

## A tile

Three sizes, cycled with `s` (the Table's sort key, which Wall does not need). The default is standard.

| Size | Rows | Content |
| --- | --- | --- |
| compact | 2 | The label and the state (with the `●` or `○`), and `+N` for the queue; then Now (the Objective when Now is empty). |
| standard | 4 | The label and the state; Now; `N queued: the next item`, or `no queue`; the directory and, when it fits, the context left. |
| roomy | 6 | The label and the state; Objective; Progress; Now; the queue row; the directory and context. |

A text that does not fit is cut with an ellipsis. Between tile rows there is one blank row (none in compact). The first two columns of a tile are the gutter, which holds the `▶` and the bar of the selected tile, as in Watch. The colours are the ones of the other tabs: the needs-you colour (until it fades), the recent-activity colour, and the plain `●` and `○` marks in the mono theme.

## Keys

On Wall `h`, `l`, `left` and `right` move between tiles and `j`, `k`, `up` and `down` move between tile rows, since the sessions are laid out in two directions. Switching tab is `tab` and `shift+tab`. So that nobody is stuck on the Wall who walked in with the arrow keys, `left` on the first tile goes to the previous tab and `right` on the last tile to the next, and an arrow key that brings you to the Wall selects the tile at the edge you came from (the first after `right`, the last after `left`), so that the same arrow key back walks out at once. `gg`, `G`, `home`, `end`, `ctrl+d`, `ctrl+u`, `pgdn` and `pgup` work as elsewhere (a page is the tile rows that fit). Everything else is as on the other tabs: `enter` jumps, `v` opens the detail view, `z` snoozes, `n` names, `/` filters, `a` shows ended sessions, `1`-`9` take an inbox item, and a click selects a tile and a double click jumps (with `--mouse`). `--tab wall` opens on it, and `sitrep status --tab wall` prints one frame.

## What the tab line shows

One summary line (the counts of Table, and the inbox), then the tiles.

## Not proposed

Tiles of different heights (masonry), a setting for the tile width, and a different order than state. They can come if real use asks.

## Tests

The layout is checked at every width from 1 to 150 and heights from 3 to 40: no line wider than the screen, every tile of one size equally high, the selected tile on screen, the counts of `+N above` and `+N more` right, and no tile cut in the middle of a row. Navigation is tested for the grid (left and right at the edges, up and down into a short last row). The mouse is tested with tile cells, and a golden file fixes the look of each size.

## Rollback

Remove the tab: nothing else depends on it. The other tabs are not changed.

# Dashboard keys, version 0.7.0

Status: first round built (0.7.0), later rounds not. Reviewed once. `q` going back from a sub-screen is already done (0.6.0). Changes from the review: `ctrl+f` and `ctrl+b` are dropped (tmux takes `ctrl+b`), `O`, `dd` and `yy` wait, `ctrl+d` keeps the page size it has, per-screen help is a later round, and the first round is the five items under First Round.

## Purpose

The dashboard's keys grew one feature at a time, and the same letter now means different things on different screens. This document states the conventions the keys follow, lists the keys as they are, and recommends a keymap that borrows from vi for movement and editing and from top and htop for the list itself.

## Conventions

1. One meaning for a key on every screen, unless the screen itself differs (a list against a line of text). Where a key does differ, the footer of that screen shall show it.
2. Movement is vi's, everywhere there is a list: `j` `k` and the arrows, `g` `g` and `G` for the first and last row, `ctrl+d` `ctrl+u` for half a page and `ctrl+f` `ctrl+b` for a page.
3. A change to a queue is vi's too: open a line (`o`, `O`), delete a line (`dd`), yank a line (`yy`), change a line (`enter`). Anything that deletes asks first, as it does now.
4. The list itself follows top: `q` quits, `?` helps, `/` filters, `s` sorts, `ctrl+l` redraws, and `r` refreshes. Keys that rewrite state use a capital letter or two keys, so a slip cannot do it.
5. `ctrl+c` quits from everywhere, and `esc` always means cancel or go back one step. `q` goes back from a sub-screen and quits from the tabs.

## The Keys Today

| Screen | Keys |
|---|---|
| Watch and Top | `q` quit, `tab` `shift+tab` `←` `→` `1` `2` tab, `j k ↑ ↓ home end` move, `enter` jump, `y` copy id, `Y` copy resume command, `z` snooze, `/` filter, `n` name, `i` inbox, `e` queue, `p` prune, `d` `space` detail, `a` ended, `s` sort (Top), `r` refresh, `?` help, `esc` clear filter |
| Detail | `q` `esc` `d` `backspace` back, `enter` `y` `Y` `z`, `j k ↑ ↓` previous or next session, `space` `pgdn` `ctrl+d` and `pgup` `b` `ctrl+u` scroll, `home` `end` |
| Queue | `q` `esc` `backspace` back, `i` `e` the other queue, `j k ↑ ↓ home end` move, `a` add, `E` edit, `x` remove (asks), `K` `J` move, `T` to the top, `t` take (inbox), `P` pin, `U` unpin |
| Line, question, filter | `enter` `esc` `backspace`, `y` `n` `q` on a question |

## What Is Wrong With Them

1. `a` adds an item in a queue and shows ended sessions on the tabs.
2. `t` takes an inbox item and `T` moves an item to the top, one shift apart, and only `t` asks.
3. Only the detail view pages. The tabs and the queue have `home` and `end` and nothing between.
4. The tab footer lists five keys, of which `s sort` does nothing on Watch, and leaves out `enter`, `/`, `n`, `z`, `i` and `e`.
5. The help is one flat list in which the screens' different keys look like one set.
6. Nothing in the queue screen yanks, and `enter` does nothing there.

## First Round

The review cut the first round to what can be built without a new help state or a table of keys. In order:

1. `g` `g` and `G` go to the first and last row of every list, and to the top and bottom of the detail view, as `home` and `end` do. A lone `g` is held by one handler, ahead of the per-screen keys. The next key completes it or, if it is not `g`, is swallowed (`g` then `x` does nothing, as in vim). `esc` clears it, and so does any change of screen. There is no timeout, as in less and htop. The status bar shows the pending key (`g-`) while it waits.
2. The footers say what works on that screen. The tab footer drops `s sort` on Watch and lists `enter jump`, `/ filter` and `n name`, and `? help` stays last. Every footer is trimmed by priority, as `statusBar` is, so a narrow screen loses the least useful hint and never `? help`.
3. In the queue, `o` adds an item at the end, and `enter` edits the selected one. `a` and `E` stay as the same keys, so what is learned now still works. (`O`, add at the front, needs an add that takes a position, as two commands cannot hold the queue lock together.)
4. `h` and `l` switch tab, beside `←` `→` `tab` `shift+tab` `1` `2`. `ctrl+d` and `ctrl+u` page the lists and the queue by the same page that `ctrl+d` has in the detail view (a screen less a line), and `pgdn` and `pgup` do the same.
5. `?` opens the help from every screen. The help state is checked first, and any key closes it, so a queue or a detail view that was open is open again after it.

## Later

1. A help with a section for each screen, which wants a scrolling help (it is cut at the screen's height today) and a table of keys that the dispatch and the help both read.
2. `d` `d` and `y` `y`. `x` already removes and asks, and `dd` is neither safer nor shorter. `yy` clashes with `y` answering yes to a question, for a user who types `dd` and then `yy` out of habit.
3. `O`, as above. A `:` line, `u` undo, `<` `>` `R` for the Top sort and `ctrl+l` (it repeats `r`) are possible, and none is needed yet.

## Not Planned

`ctrl+f` and `ctrl+b` (tmux takes `ctrl+b`, and `pgdn` and `pgup` page), number prefixes such as `5j` (the keys `1` and `2` are tabs), `H` `M` `L` and `zz`, top's `k` for kill and `W` for writing a configuration.

## Exceptions To The Conventions

1. In the detail view `j` `k` and the arrows change the session, and the page keys scroll. `g` `g` and `G` scroll, as `home` and `end` do.
2. `space` and `d` open the detail view on the tabs and page or close it in the detail view, as vi and less use `space` for forward.
3. `enter` jumps on the tabs and edits in the queue screen, and `y` copies on the tabs and answers a question in the queue screen. A screen's footer shows what its keys do.

## Tests

A test presses each documented key on its screen and checks what happens, in place of a table of keys that does not exist. The new footers are drawn at the widths the tests use (1, 2, 10, 20, 30 and 80 columns) and none may be wider than the screen. The help text, the README and `sitrep help watch` change with the keys.

## Rollback Cost

Every key that exists today keeps working in this recommendation, except `q` in a sub-screen, which changed in 0.6.0, so a user of 0.6.0 loses nothing by it. New keys can be taken out without affecting the old ones.

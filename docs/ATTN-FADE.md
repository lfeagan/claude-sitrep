# Fading the needs-you colour

Status: built (0.16.0).

## Claim

A session that finished its turn is "waiting" and shows the `attn` colour for as long as it sits, so a dashboard with a few forgotten sessions is mostly yellow (in the rainbow theme) and the colour stops meaning "look here now". The colour should say that something needs the user lately, and fade when a session has waited a long time.

## Rule

A session that needs the user (`needs_input`) is painted with `attn` only for `SITREP_ATTN_FADE` seconds, counted from when it began to wait: the time of its last state change, or for a session at a permission dialog the time the dialog opened. After that it is drawn without colour.

- `SITREP_ATTN_FADE=<seconds>`, default 600 (ten minutes, the time after which a working session is called `idle?`). `0` never fades, as before. A value that is not a number is the default.
- Only the colour fades. `needs_input` stays true, so the `●` mark, the Needs You count, the sort order, the bell (which rings on the change to needing the user) and the `needs_input` field of the JSON are unchanged. A program that reads the JSON is not affected.
- A snooze is unchanged and still shows the hollow `○`.
- The snapshot says it: a new field `attn_faded` (true only when `needs_input` is true and the time has passed; left out otherwise). The snapshot owns the rule so that every reader uses one time, and the dashboard needs no clock.
- The CTX cell turns its own warning colour when the window is nearly full and no row colour is painted. A faded row is not painted, so it may show that warning again.

## Rollback

`SITREP_ATTN_FADE=0` restores the old behaviour. The field is additive.

## Not proposed

A second, dimmer stage of colour, and a fade of the `●` mark.

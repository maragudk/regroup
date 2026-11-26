# Configurable accent colour

Let the accent colour be chosen from Tailwind's default palette.

Markus on 2026-11-26: "we may want to have the accent color configurable too, from the TailwindCSS default palette".

## Why

Orange is the default, and we use Tailwind's default palette so that we do not have to invent colours. Choosing another hue from the same palette changes the look without changing the layout.

Fonts are different. Markus first suggested making the app-wide font switchable, then withdrew it the same day: "I may want to walk back the font choice possibility, since it can change the layout drastically in a way color selection can't." So regroup ships one serif chosen for the design, and fonts are not configurable.

## Open

- How it is chosen: in the host's configuration, or by an admin inside the app. One instance holds one group, so either way it applies to the whole group.
- Which hues are offered. Some, such as yellow and lime, are too light to carry text or white labels, so each offered hue needs checking for contrast.
- Nothing is designed yet. This is a later piece of work.

## Consequence for work before then

Pages use the `primary` colour names from `tailwind.css`, which point at orange, and never name orange directly. Then the accent can change in one place.

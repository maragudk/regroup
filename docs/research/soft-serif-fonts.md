# Soft serif typefaces for the display type

Research note, 2026-11-26.

## What was asked, and why

Regroup sets its display type in a soft, friendly serif: the group's name at about 90px on the orange band, day headings ("Today", "Yesterday") at about 64px, and people's names at about 36px and 20px in dark orange on white. Body text is a separate sans and is not covered here.

Markus's earlier design used Recoleta, which he describes as "70's soft vibes". Recoleta blends the round softness of Cooper with the angled strokes of Windsor. It is a commercial Latinotype font and cannot ship in an MIT-licensed public repository. He has asked for "a nice serif font for our design instead", and has walked back the idea of making the app-wide font switchable, so this one face carries the whole look. The current stand-in is Young Serif.

Each candidate had to meet one hard requirement: a licence that allows the font files to be bundled in a public MIT-licensed repository and served from any self-hosted instance. The SIL Open Font License 1.1 (OFL) meets it. The facts below come from each font's Google Fonts metadata (`METADATA.pb` in [google/fonts](https://github.com/google/fonts)), its own repository, and the Google Fonts CSS API. No font files were downloaded.

A note on the OFL: it allows bundling, redistributing and serving the fonts with any software, including commercial or MIT-licensed software, as long as the fonts themselves stay under the OFL and are not sold on their own. Ship the font's `OFL.txt` next to the files. A font with a Reserved Font Name must be renamed if it is modified, and subsetting can count as modification, so for such a font the safest course is to serve the unmodified files.

A note on Danish: Google Fonts serves each family below with its `latin` and `latin-ext` subsets. The `latin` subset covers U+0000 to U+00FF, which includes æ ø å Æ Ø Å. For Cooper*, which is not on Google Fonts, the glyphs were confirmed in its FontForge source.

## Fraunces

- Licence: OFL 1.1, with no Reserved Font Name. [OFL.txt](https://github.com/google/fonts/blob/main/ofl/fraunces/OFL.txt), [repository](https://github.com/undercasetype/Fraunces).
- Files: Google Fonts family `Fraunces`. Upstream at [github.com/undercasetype/Fraunces](https://github.com/undercasetype/Fraunces).
- Weights and axes: one variable font with four axes, upright and italic: `wght` 100 to 900, `opsz` 9 to 144, `SOFT` 0 to 100 (Sharp, Soft, SuperSoft), and `WONK` 0 to 1. A Google Fonts URL that sets all four axes, such as `family=Fraunces:ital,opsz,wght,SOFT,WONK@0,9..144,100..900,0..100,0..1;1,9..144,100..900,0..100,0..1`, is accepted. Upstream defaults are opsz 144, wght 900, SOFT 100, WONK 1.
- Language: latin, latin-ext, vietnamese.
- Character: Undercase Type describe it as an "Old Style soft-serif typeface inspired by the mannerisms of early 20th century typefaces such as Windsor, Souvenir, and the Cooper Series". That is the same pair of sources as Recoleta. `SOFT` 100 rounds every serif and terminal. `WONK` 1 brings in Windsor's leaning n, m and h, which is the angled note in Recoleta. The optical size axis does what the four sizes need: at 90px and 64px (opsz 144) it has tall contrast and tight spacing, and at 20px (opsz near 20) it gets a larger x-height, wider letters and looser spacing. The `WONK` forms switch off on their own below opsz 18. With `SOFT` 100, `WONK` 1 and a weight of 500 to 700 it comes closest to Recoleta of all the candidates here. Its quirks can tip into caricature at weight 900. The one cost is file size: the full variable font is large, so bundle only the axes and ranges the design uses, or a few static instances.

## Cooper* (Indestructible Type)

- Licence: OFL 1.1, with no Reserved Font Name. [LICENSE.md](https://github.com/indestructible-type/Cooper/blob/main/LICENSE.md), [repository](https://github.com/indestructible-type/Cooper). GitHub reports the licence as `OFL-1.1`.
- Files: not on Google Fonts. The repository ships ready-made `.woff2` files in [`fonts/webfonts/`](https://github.com/indestructible-type/Cooper/tree/main/fonts/webfonts), along with `.ttf`, `.otf` and variable fonts. Project page: [indestructibletype.com/Cooper](https://indestructibletype.com/Cooper/).
- Weights and axes: static Regular, Medium, SemiBold, Bold, ExtraBold and Black, each with an italic, plus variable `Cooper[wght].ttf` and `Cooper-Italic[wght].ttf` (weight axis only).
- Language: the regular source has Aring, aring, AE, ae, Oslash and oslash, as well as Central European letters such as ccaron, lslash, eogonek and uni0219. Danish is covered. The exact Latin Extended range was not counted.
- Character: a revival by Owen Earl of Oswald Cooper's whole 1920s series, not just Cooper Black. The README calls the regular weight "organic, Art Nouveau... hardly a straight line or a sharp corner", legible even at text sizes. It is the Cooper half of Recoleta's ancestry, without the Windsor lean, so it reads softer and more bubbly than Recoleta. SemiBold or Bold should suit 90px and 64px, and Medium suits 36px and 20px. It lacks optical sizes, but the regular weights were drawn for text, so 20px should be fine. Last pushed May 2025. Being outside Google Fonts, it has had less outside quality control.

## Caprasimo

- Licence: OFL 1.1, with no Reserved Font Name. [OFL.txt](https://github.com/google/fonts/blob/main/ofl/caprasimo/OFL.txt), [repository](https://github.com/docrepair-fonts/caprasimo-fonts).
- Files: Google Fonts family `Caprasimo`.
- Weights and axes: one static weight, 400, which is very heavy. No italic.
- Language: latin, latin-ext.
- Character: a metric-compatible stand-in for Cooper Black, derived from Fraunces by Undercase Type. Round and fat, unmistakably 1970s. Only one weight, and a black one: fine for the 90px group name, heavy at 64px, and too dense for names at 20px. It only works as a single display accent, and would force a second face for the smaller sizes.

## Young Serif (current stand-in)

- Licence: OFL 1.1, with no Reserved Font Name. [OFL.txt](https://github.com/google/fonts/blob/main/ofl/youngserif/OFL.txt), [repository](https://github.com/noirblancrouge/YoungSerif).
- Files: Google Fonts family `Young Serif`.
- Weights and axes: one static weight, 400 (drawn heavy). No italic.
- Language: latin, latin-ext.
- Character: Google Fonts describes it as "a heavy weight old style serif typeface, taking inspiration from Plantin Infant or ITC Italian Old Style", with rounded b and f. Warm and friendly, but more bookish than 1970s, with none of Recoleta's lean or bounce. Its single weight cannot set up a hierarchy: the group name, the day headings and the names all come out the same weight, and it is thick at 20px.

## Corben

- Licence: OFL 1.1, with the Reserved Font Name "Corben". [OFL.txt](https://github.com/google/fonts/blob/main/ofl/corben/OFL.txt), [upstream](https://github.com/librefonts/corben). Because of the Reserved Font Name, serve the unmodified files rather than a subset under the same name.
- Files: Google Fonts family `Corben`, weights 400 and 700.
- Weights and axes: static 400 and 700. No italic.
- Language: latin, latin-ext.
- Character: Vernon Adams's 2010 display face, "ample curves and ligatures... a touch of classic display lettering". Cooper-like and round. The bold is warm at 64px and 90px. It is older, simpler work, with less refined spacing and kerning, which shows at 90px. The regular is usable at 20px. A fallback rather than a first choice.

## Gloock (considered, not recommended)

- Licence: OFL 1.1, with no Reserved Font Name. [OFL.txt](https://github.com/google/fonts/blob/main/ofl/gloock/OFL.txt), [repository](https://github.com/duartp/gloock).
- Files: Google Fonts family `Gloock`. One weight, 400. No italic. Subsets latin, latin-ext, cyrillic-ext.
- Character: "a contemporary high-contrast serif typeface intended for display use... inspired by newspaper's headlines". Sharp hairlines, not soft, and nothing of the 1970s. It is elegant at 90px, but the thin strokes would get fragile in orange on white at 20px. It does not fit the brief.

## Excluded

- Fontshare's closed-source fonts are under the ITF Free Font License ([licence page](https://www.fontshare.com/licenses/itf-ffl)). It allows free personal and commercial use and self-hosting "for use on your own websites and applications". It also requires third parties to get their own copy from Fontshare, and third-party summaries say it forbids modification, including subsetting and format conversion. Bundling the files in a public repository hands copies to every self-hoster, so these fonts are out. Fontshare's OFL fonts would be fine, but no OFL serif there matched the brief.
- Bree Serif and Coustard (both OFL, on Google Fonts) are soft slab serifs, friendly but not 1970s. They are left out so as not to pad the list.
- Recoleta, and look-alikes such as Gazpacho and Roca, are commercial.

## Recommendation

1. Fraunces. It is the only candidate built from Recoleta's own two sources: Cooper's softness through `SOFT`, and Windsor's angle through `WONK`. It has the full weight range, an italic, and an optical size axis that covers both 90px and 20px in one family. A suggested starting point is `SOFT` 100 and `WONK` 1, weight about 600 for the group name and day headings, and about 500 for names, with `opsz` set automatically (`font-optical-sizing: auto`).
2. Cooper*. The best choice if Markus wants rounder and bubblier, with less lean. It has six weights and italics, ships its own woff2 files, and is OFL.
3. Corben. An acceptable fallback with two weights, but dated craft.
4. Caprasimo. Only for the 90px group name, paired with another face for the smaller sizes, which goes against having one face.
5. Young Serif. Warm, but a single heavy weight that is more bookish than 1970s. It should be replaced.
6. Gloock. Not soft, so not a fit.

## What could not be verified

- How each face looks at the four sizes is judged from specimens and the designers' descriptions, not from renders in Regroup. A side-by-side render of Fraunces and Cooper* at 90, 64, 36 and 20px would settle it.
- For the Google Fonts families, Danish coverage is inferred from the `latin` subset (U+0000 to U+00FF) that Google Fonts serves, not checked glyph by glyph.
- That the ITF Free Font License forbids modification comes from third-party summaries. The licence page would not render for automated fetching.

# bomexpo

![release](https://img.shields.io/github/v/release/antaresrvish/bomexpo) ![downloads](https://img.shields.io/github/downloads/antaresrvish/bomexpo/total) [![homebrew](https://img.shields.io/badge/homebrew-antaresrvish%2Ftap-informational)](https://github.com/antaresrvish/homebrew-tap) ![AUR](https://img.shields.io/aur/version/bomexpo-bin) ![license](https://img.shields.io/github/license/antaresrvish/bomexpo)

Turn a KiCad board into a JLCPCB assembly order from the terminal. Point bomexpo at a
`.kicad_pcb`, assign an LCSC part to each component, and export the BOM, CPL and Gerbers
as one order-ready zip.

Everything comes from the `.kicad_pcb` itself — components, values, placements and the
board outline.

![bomexpo Components view](docs/components-view.png)

## Install

Homebrew:

```sh
brew tap antaresrvish/tap
brew trust antaresrvish/tap
brew install bomexpo
```

Homebrew 6 won't load a formula from a third-party tap until you trust it — that's the
`brew trust` line, and it's a one-time step.

Arch (AUR):

```sh
yay -S bomexpo-bin
```

Otherwise grab a binary from the [releases page](https://github.com/antaresrvish/bomexpo/releases).
A winget package is in review.

## Usage

```sh
bomexpo path/to/board.kicad_pcb
# a project folder or .kicad_pro works too
bomexpo ~/designs/drone
```



Components start unassigned. Search LCSC to pick a part, or press `a` to auto-assign by
value, package and type. As you move through the table, the side panel shows the selected
part's stock, price and specs next to a live board preview. Once everything is assigned
and in stock, the Check tab writes the zip you upload to JLCPCB.

In the Parts tab, `/` opens a popup over the results: the same query on top, and every
category your results fall into below, boxed and grouped the way the source groups them.
Arrows pick one, `enter` narrows the table to it. The boxes come from the results, not a catalogue —
neither LCSC nor JLCPCB will search inside a category, but both label every part with one.

Common keys: `enter` assign · `a` auto-assign · `o` cycle rotation override · `x` exclude
· `w` write LCSC codes back to the pcb · `t`/`b`/`i` open a 3D render · `r` refresh stock
· `n` filter by net.

### Moving around

Whatever has the keyboard is marked with `▸`. A text field keeps every key while it's
focused, and `tab` is how you take it back — so a search box never swallows a command
again. Once a list has focus the letters are the commands (`p` pin, `d` datasheet, `s`
in-stock only, `/` back to typing), and `[` `]` or `1`–`5` switch tabs. Arrow keys always
drive the list, focused or not.

Landing on a tab never takes the keyboard, so tab switching keeps working; press `/` or
`tab` when you want to type. On the Load screen `↓` walks into the directory listing and
`enter` opens what's highlighted. On Check the issue list and the board both want the
arrows, so `tab` walks the three panes — issues, board, output path — and `↑↓` goes to
whichever has the keyboard.


## What it does

- Live LCSC stock, unit price and volume pricing per part.
- Auto-assign that matches value, package, tolerance, voltage and dielectric, and skips
  out-of-stock parts.
- Footprint rotation corrected for JLCPCB's pick-and-place, with a per-part override you
  can save.
- Honours KiCad DNP and exclude-from-BOM flags, and warns when an assigned value drifts
  from the schematic.
- Writes assignments back into the `.kicad_pcb`, so they persist and travel with the
  design in git.
- Exports BOM + CPL + Gerbers in a single zip.

Gerber/CPL export and the 3D render use `kicad-cli`, which ships with KiCad.

## Images
![Load screen](docs/load-view.png)
![bomexpo Components view](docs/components-view.png)
![Check tab](docs/check-view.png)

## Build from source

```sh
go build -o bomexpo .
```

## License

MIT — see [LICENSE](LICENSE).

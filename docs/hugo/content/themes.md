---
title: "Themes"
weight: 6
---

# Themes

thinkt ships with 14 built-in themes. Browse them interactively with `thinkt theme browse`, or switch with `thinkt theme set <name>`.

You can also [import iTerm2 color schemes](#importing-iterm2-color-schemes) to create your own.

{{< hint info >}}
**Tip:** Use `thinkt theme browse` to preview themes live in your terminal before committing to one.
{{< /hint >}}

---

## Built-in Themes

Each palette shows: accent, borders, text colors, label colors, and block backgrounds.

<style>
.theme-card { margin-bottom: 2rem; }
.theme-card h3 { margin-bottom: 0.25rem; }
.theme-card p { margin-top: 0; color: #888; font-size: 0.9em; }
.theme-card code { font-size: 0.85em; }
.palette { display: flex; gap: 2px; border-radius: 6px; overflow: hidden; height: 40px; margin-bottom: 4px; }
.palette .swatch { flex: 1; min-width: 0; }
.palette-labels { display: flex; gap: 2px; height: 28px; border-radius: 4px; overflow: hidden; margin-bottom: 4px; }
.palette-labels .swatch { flex: 1; min-width: 0; }
.palette-blocks { display: flex; gap: 2px; height: 28px; border-radius: 4px; overflow: hidden; }
.palette-blocks .swatch { flex: 1; min-width: 0; }
.palette-row-label { font-size: 0.75em; color: #999; margin-bottom: 2px; margin-top: 6px; }
</style>

<!-- dark -->
<div class="theme-card">

### dark

Default dark theme — `thinkt theme set dark`

<div class="palette-row-label">chrome &amp; text</div>
<div class="palette">
  <div class="swatch" style="background:#7D56F4" title="accent"></div>
  <div class="swatch" style="background:#7D56F4" title="border active"></div>
  <div class="swatch" style="background:#444444" title="border inactive"></div>
  <div class="swatch" style="background:#ffffff" title="text primary"></div>
  <div class="swatch" style="background:#888888" title="text secondary"></div>
  <div class="swatch" style="background:#666666" title="text muted"></div>
</div>
<div class="palette-row-label">labels</div>
<div class="palette-labels">
  <div class="swatch" style="background:#5dade2" title="user label"></div>
  <div class="swatch" style="background:#58d68d" title="assistant label"></div>
  <div class="swatch" style="background:#af7ac5" title="thinking label"></div>
  <div class="swatch" style="background:#f0b27a" title="tool label"></div>
  <div class="swatch" style="background:#ff87d7" title="confirm selected"></div>
</div>
<div class="palette-row-label">block backgrounds</div>
<div class="palette-blocks">
  <div class="swatch" style="background:#1a3a5c" title="user block"></div>
  <div class="swatch" style="background:#1a3c1a" title="assistant block"></div>
  <div class="swatch" style="background:#3a1a3c" title="thinking block"></div>
  <div class="swatch" style="background:#3c2a1a" title="tool call block"></div>
  <div class="swatch" style="background:#1a2a3c" title="tool result block"></div>
</div>
</div>

<!-- light -->
<div class="theme-card">

### light

Light theme for bright terminals — `thinkt theme set light`

<div class="palette-row-label">chrome &amp; text</div>
<div class="palette">
  <div class="swatch" style="background:#6941C6" title="accent"></div>
  <div class="swatch" style="background:#6941C6" title="border active"></div>
  <div class="swatch" style="background:#d0d0d0" title="border inactive"></div>
  <div class="swatch" style="background:#1a1a1a" title="text primary"></div>
  <div class="swatch" style="background:#666666" title="text secondary"></div>
  <div class="swatch" style="background:#999999" title="text muted"></div>
</div>
<div class="palette-row-label">labels</div>
<div class="palette-labels">
  <div class="swatch" style="background:#1976d2" title="user label"></div>
  <div class="swatch" style="background:#388e3c" title="assistant label"></div>
  <div class="swatch" style="background:#8e24aa" title="thinking label"></div>
  <div class="swatch" style="background:#f57c00" title="tool label"></div>
  <div class="swatch" style="background:#6941C6" title="confirm selected"></div>
</div>
<div class="palette-row-label">block backgrounds</div>
<div class="palette-blocks">
  <div class="swatch" style="background:#e3f2fd" title="user block"></div>
  <div class="swatch" style="background:#e8f5e9" title="assistant block"></div>
  <div class="swatch" style="background:#f3e5f5" title="thinking block"></div>
  <div class="swatch" style="background:#fff3e0" title="tool call block"></div>
  <div class="swatch" style="background:#e0f2f1" title="tool result block"></div>
</div>
</div>

<!-- dracula -->
<div class="theme-card">

### dracula

Dracula color scheme — `thinkt theme set dracula`

<div class="palette-row-label">chrome &amp; text</div>
<div class="palette">
  <div class="swatch" style="background:#d6acff" title="accent"></div>
  <div class="swatch" style="background:#d6acff" title="border active"></div>
  <div class="swatch" style="background:#6272a4" title="border inactive"></div>
  <div class="swatch" style="background:#f8f8f2" title="text primary"></div>
  <div class="swatch" style="background:#6272a4" title="text secondary"></div>
  <div class="swatch" style="background:#4b5578" title="text muted"></div>
</div>
<div class="palette-row-label">labels</div>
<div class="palette-labels">
  <div class="swatch" style="background:#bd93f9" title="user label"></div>
  <div class="swatch" style="background:#50fa7b" title="assistant label"></div>
  <div class="swatch" style="background:#ff79c6" title="thinking label"></div>
  <div class="swatch" style="background:#f1fa8c" title="tool label"></div>
  <div class="swatch" style="background:#d6acff" title="confirm selected"></div>
</div>
<div class="palette-row-label">block backgrounds</div>
<div class="palette-blocks">
  <div class="swatch" style="background:#3e3a53" title="user block"></div>
  <div class="swatch" style="background:#2e4940" title="assistant block"></div>
  <div class="swatch" style="background:#423347" title="thinking block"></div>
  <div class="swatch" style="background:#404340" title="tool call block"></div>
  <div class="swatch" style="background:#34414e" title="tool result block"></div>
</div>
</div>

<!-- nord -->
<div class="theme-card">

### nord

Nord color scheme — `thinkt theme set nord`

<div class="palette-row-label">chrome &amp; text</div>
<div class="palette">
  <div class="swatch" style="background:#81a1c1" title="accent"></div>
  <div class="swatch" style="background:#81a1c1" title="border active"></div>
  <div class="swatch" style="background:#4c566a" title="border inactive"></div>
  <div class="swatch" style="background:#d8dee9" title="text primary"></div>
  <div class="swatch" style="background:#4c566a" title="text secondary"></div>
  <div class="swatch" style="background:#404859" title="text muted"></div>
</div>
<div class="palette-row-label">labels</div>
<div class="palette-labels">
  <div class="swatch" style="background:#81a1c1" title="user label"></div>
  <div class="swatch" style="background:#a3be8c" title="assistant label"></div>
  <div class="swatch" style="background:#b48ead" title="thinking label"></div>
  <div class="swatch" style="background:#ebcb8b" title="tool label"></div>
  <div class="swatch" style="background:#81a1c1" title="confirm selected"></div>
</div>
<div class="palette-row-label">block backgrounds</div>
<div class="palette-blocks">
  <div class="swatch" style="background:#3a4453" title="user block"></div>
  <div class="swatch" style="background:#40494b" title="assistant block"></div>
  <div class="swatch" style="background:#3e3f4d" title="thinking block"></div>
  <div class="swatch" style="background:#454649" title="tool call block"></div>
  <div class="swatch" style="background:#394551" title="tool result block"></div>
</div>
</div>

<!-- gruvbox-dark -->
<div class="theme-card">

### gruvbox-dark

Gruvbox dark variant — `thinkt theme set gruvbox-dark`

<div class="palette-row-label">chrome &amp; text</div>
<div class="palette">
  <div class="swatch" style="background:#83a598" title="accent"></div>
  <div class="swatch" style="background:#83a598" title="border active"></div>
  <div class="swatch" style="background:#928374" title="border inactive"></div>
  <div class="swatch" style="background:#ebdbb2" title="text primary"></div>
  <div class="swatch" style="background:#928374" title="text secondary"></div>
  <div class="swatch" style="background:#685f56" title="text muted"></div>
</div>
<div class="palette-row-label">labels</div>
<div class="palette-labels">
  <div class="swatch" style="background:#458588" title="user label"></div>
  <div class="swatch" style="background:#98971a" title="assistant label"></div>
  <div class="swatch" style="background:#b16286" title="thinking label"></div>
  <div class="swatch" style="background:#d79921" title="tool label"></div>
  <div class="swatch" style="background:#83a598" title="confirm selected"></div>
</div>
<div class="palette-row-label">block backgrounds</div>
<div class="palette-blocks">
  <div class="swatch" style="background:#2c3636" title="user block"></div>
  <div class="swatch" style="background:#393926" title="assistant block"></div>
  <div class="swatch" style="background:#382f33" title="thinking block"></div>
  <div class="swatch" style="background:#3d3627" title="tool call block"></div>
  <div class="swatch" style="background:#303630" title="tool result block"></div>
</div>
</div>

<!-- gruvbox-light -->
<div class="theme-card">

### gruvbox-light

Gruvbox light variant — `thinkt theme set gruvbox-light`

<div class="palette-row-label">chrome &amp; text</div>
<div class="palette">
  <div class="swatch" style="background:#076678" title="accent"></div>
  <div class="swatch" style="background:#076678" title="border active"></div>
  <div class="swatch" style="background:#928374" title="border inactive"></div>
  <div class="swatch" style="background:#3c3836" title="text primary"></div>
  <div class="swatch" style="background:#928374" title="text secondary"></div>
  <div class="swatch" style="background:#bcaf95" title="text muted"></div>
</div>
<div class="palette-row-label">labels</div>
<div class="palette-labels">
  <div class="swatch" style="background:#458588" title="user label"></div>
  <div class="swatch" style="background:#98971a" title="assistant label"></div>
  <div class="swatch" style="background:#b16286" title="thinking label"></div>
  <div class="swatch" style="background:#d79921" title="tool label"></div>
  <div class="swatch" style="background:#076678" title="confirm selected"></div>
</div>
<div class="palette-row-label">block backgrounds</div>
<div class="palette-blocks">
  <div class="swatch" style="background:#e0e1be" title="user block"></div>
  <div class="swatch" style="background:#ece4ad" title="assistant block"></div>
  <div class="swatch" style="background:#f2e0bf" title="thinking block"></div>
  <div class="swatch" style="background:#f7e6b3" title="tool call block"></div>
  <div class="swatch" style="background:#e9e7bc" title="tool result block"></div>
</div>
</div>

<!-- catppuccin-mocha -->
<div class="theme-card">

### catppuccin-mocha

Catppuccin Mocha (dark) — `thinkt theme set catppuccin-mocha`

<div class="palette-row-label">chrome &amp; text</div>
<div class="palette">
  <div class="swatch" style="background:#74a8fc" title="accent"></div>
  <div class="swatch" style="background:#74a8fc" title="border active"></div>
  <div class="swatch" style="background:#585b70" title="border inactive"></div>
  <div class="swatch" style="background:#cdd6f4" title="text primary"></div>
  <div class="swatch" style="background:#585b70" title="text secondary"></div>
  <div class="swatch" style="background:#414356" title="text muted"></div>
</div>
<div class="palette-row-label">labels</div>
<div class="palette-labels">
  <div class="swatch" style="background:#89b4fa" title="user label"></div>
  <div class="swatch" style="background:#a6e3a1" title="assistant label"></div>
  <div class="swatch" style="background:#f5c2e7" title="thinking label"></div>
  <div class="swatch" style="background:#f9e2af" title="tool label"></div>
  <div class="swatch" style="background:#74a8fc" title="confirm selected"></div>
</div>
<div class="palette-row-label">block backgrounds</div>
<div class="palette-blocks">
  <div class="swatch" style="background:#2e354d" title="user block"></div>
  <div class="swatch" style="background:#323c3f" title="assistant block"></div>
  <div class="swatch" style="background:#383244" title="thinking block"></div>
  <div class="swatch" style="background:#38363d" title="tool call block"></div>
  <div class="swatch" style="background:#2c3642" title="tool result block"></div>
</div>
</div>

<!-- catppuccin-latte -->
<div class="theme-card">

### catppuccin-latte

Catppuccin Latte (light) — `thinkt theme set catppuccin-latte`

<div class="palette-row-label">chrome &amp; text</div>
<div class="palette">
  <div class="swatch" style="background:#456e00" title="accent"></div>
  <div class="swatch" style="background:#456e00" title="border active"></div>
  <div class="swatch" style="background:#6c6f85" title="border inactive"></div>
  <div class="swatch" style="background:#4c4f69" title="text primary"></div>
  <div class="swatch" style="background:#6c6f85" title="text secondary"></div>
  <div class="swatch" style="background:#a0a3b2" title="text muted"></div>
</div>
<div class="palette-row-label">labels</div>
<div class="palette-labels">
  <div class="swatch" style="background:#1e66f5" title="user label"></div>
  <div class="swatch" style="background:#40a02b" title="assistant label"></div>
  <div class="swatch" style="background:#ea76cb" title="thinking label"></div>
  <div class="swatch" style="background:#df8e1d" title="tool label"></div>
  <div class="swatch" style="background:#456e00" title="confirm selected"></div>
</div>
<div class="palette-row-label">block backgrounds</div>
<div class="palette-blocks">
  <div class="swatch" style="background:#d0dcf5" title="user block"></div>
  <div class="swatch" style="background:#d5e5d7" title="assistant block"></div>
  <div class="swatch" style="background:#eee2f0" title="thinking block"></div>
  <div class="swatch" style="background:#ede5db" title="tool call block"></div>
  <div class="swatch" style="background:#d5e6ea" title="tool result block"></div>
</div>
</div>

<!-- solarized-dark -->
<div class="theme-card">

### solarized-dark

Solarized dark variant — `thinkt theme set solarized-dark`

<div class="palette-row-label">chrome &amp; text</div>
<div class="palette">
  <div class="swatch" style="background:#708284" title="accent"></div>
  <div class="swatch" style="background:#708284" title="border active"></div>
  <div class="swatch" style="background:#475b62" title="border inactive"></div>
  <div class="swatch" style="background:#708284" title="text primary"></div>
  <div class="swatch" style="background:#475b62" title="text secondary"></div>
  <div class="swatch" style="background:#2b434a" title="text muted"></div>
</div>
<div class="palette-row-label">labels</div>
<div class="palette-labels">
  <div class="swatch" style="background:#2176c7" title="user label"></div>
  <div class="swatch" style="background:#738a05" title="assistant label"></div>
  <div class="swatch" style="background:#c61c6f" title="thinking label"></div>
  <div class="swatch" style="background:#a57706" title="tool label"></div>
  <div class="swatch" style="background:#708284" title="confirm selected"></div>
</div>
<div class="palette-row-label">block backgrounds</div>
<div class="palette-blocks">
  <div class="swatch" style="background:#052b3f" title="user block"></div>
  <div class="swatch" style="background:#112e22" title="assistant block"></div>
  <div class="swatch" style="background:#181e30" title="thinking block"></div>
  <div class="swatch" style="background:#142923" title="tool call block"></div>
  <div class="swatch" style="background:#042c32" title="tool result block"></div>
</div>
</div>

<!-- solarized-light -->
<div class="theme-card">

### solarized-light

Solarized light variant — `thinkt theme set solarized-light`

<div class="palette-row-label">chrome &amp; text</div>
<div class="palette">
  <div class="swatch" style="background:#839496" title="accent"></div>
  <div class="swatch" style="background:#839496" title="border active"></div>
  <div class="swatch" style="background:#002b36" title="border inactive"></div>
  <div class="swatch" style="background:#657b83" title="text primary"></div>
  <div class="swatch" style="background:#002b36" title="text secondary"></div>
  <div class="swatch" style="background:#657c7b" title="text muted"></div>
</div>
<div class="palette-row-label">labels</div>
<div class="palette-labels">
  <div class="swatch" style="background:#268bd2" title="user label"></div>
  <div class="swatch" style="background:#859900" title="assistant label"></div>
  <div class="swatch" style="background:#d33682" title="thinking label"></div>
  <div class="swatch" style="background:#b58900" title="tool label"></div>
  <div class="swatch" style="background:#839496" title="confirm selected"></div>
</div>
<div class="palette-row-label">block backgrounds</div>
<div class="palette-blocks">
  <div class="swatch" style="background:#dde6e0" title="user block"></div>
  <div class="swatch" style="background:#ebe8c1" title="assistant block"></div>
  <div class="swatch" style="background:#f8dfd7" title="thinking block"></div>
  <div class="swatch" style="background:#f4e9c8" title="tool call block"></div>
  <div class="swatch" style="background:#e4ecda" title="tool result block"></div>
</div>
</div>

<!-- tokyo-night -->
<div class="theme-card">

### tokyo-night

Tokyo Night color scheme — `thinkt theme set tokyo-night`

<div class="palette-row-label">chrome &amp; text</div>
<div class="palette">
  <div class="swatch" style="background:#7aa2f7" title="accent"></div>
  <div class="swatch" style="background:#7aa2f7" title="border active"></div>
  <div class="swatch" style="background:#414868" title="border inactive"></div>
  <div class="swatch" style="background:#c0caf5" title="text primary"></div>
  <div class="swatch" style="background:#414868" title="text secondary"></div>
  <div class="swatch" style="background:#31364e" title="text muted"></div>
</div>
<div class="palette-row-label">labels</div>
<div class="palette-labels">
  <div class="swatch" style="background:#7aa2f7" title="user label"></div>
  <div class="swatch" style="background:#9ece6a" title="assistant label"></div>
  <div class="swatch" style="background:#bb9af7" title="thinking label"></div>
  <div class="swatch" style="background:#e0af68" title="tool label"></div>
  <div class="swatch" style="background:#7aa2f7" title="confirm selected"></div>
</div>
<div class="palette-row-label">block backgrounds</div>
<div class="palette-blocks">
  <div class="swatch" style="background:#282f45" title="user block"></div>
  <div class="swatch" style="background:#2e3630" title="assistant block"></div>
  <div class="swatch" style="background:#2d2a3f" title="thinking block"></div>
  <div class="swatch" style="background:#322d2e" title="tool call block"></div>
  <div class="swatch" style="background:#263140" title="tool result block"></div>
</div>
</div>

<!-- rose-pine -->
<div class="theme-card">

### rose-pine

Rose Pine color scheme — `thinkt theme set rose-pine`

<div class="palette-row-label">chrome &amp; text</div>
<div class="palette">
  <div class="swatch" style="background:#9ccfd8" title="accent"></div>
  <div class="swatch" style="background:#9ccfd8" title="border active"></div>
  <div class="swatch" style="background:#6e6a86" title="border inactive"></div>
  <div class="swatch" style="background:#e0def4" title="text primary"></div>
  <div class="swatch" style="background:#6e6a86" title="text secondary"></div>
  <div class="swatch" style="background:#4c495f" title="text muted"></div>
</div>
<div class="palette-row-label">labels</div>
<div class="palette-labels">
  <div class="swatch" style="background:#9ccfd8" title="user label"></div>
  <div class="swatch" style="background:#31748f" title="assistant label"></div>
  <div class="swatch" style="background:#c4a7e7" title="thinking label"></div>
  <div class="swatch" style="background:#f6c177" title="tool label"></div>
  <div class="swatch" style="background:#9ccfd8" title="confirm selected"></div>
</div>
<div class="palette-row-label">block backgrounds</div>
<div class="palette-blocks">
  <div class="swatch" style="background:#2d333f" title="user block"></div>
  <div class="swatch" style="background:#1d2534" title="assistant block"></div>
  <div class="swatch" style="background:#2e283b" title="thinking block"></div>
  <div class="swatch" style="background:#342b2e" title="tool call block"></div>
  <div class="swatch" style="background:#322b36" title="tool result block"></div>
</div>
</div>

<!-- one-dark -->
<div class="theme-card">

### one-dark

Atom One Dark color scheme — `thinkt theme set one-dark`

<div class="palette-row-label">chrome &amp; text</div>
<div class="palette">
  <div class="swatch" style="background:#61afef" title="accent"></div>
  <div class="swatch" style="background:#61afef" title="border active"></div>
  <div class="swatch" style="background:#767676" title="border inactive"></div>
  <div class="swatch" style="background:#abb2bf" title="text primary"></div>
  <div class="swatch" style="background:#767676" title="text secondary"></div>
  <div class="swatch" style="background:#545658" title="text muted"></div>
</div>
<div class="palette-row-label">labels</div>
<div class="palette-labels">
  <div class="swatch" style="background:#61afef" title="user label"></div>
  <div class="swatch" style="background:#98c379" title="assistant label"></div>
  <div class="swatch" style="background:#c678dd" title="thinking label"></div>
  <div class="swatch" style="background:#e5c07b" title="tool label"></div>
  <div class="swatch" style="background:#61afef" title="confirm selected"></div>
</div>
<div class="palette-row-label">block backgrounds</div>
<div class="palette-blocks">
  <div class="swatch" style="background:#2b3a48" title="user block"></div>
  <div class="swatch" style="background:#333d37" title="assistant block"></div>
  <div class="swatch" style="background:#352f40" title="thinking block"></div>
  <div class="swatch" style="background:#393835" title="tool call block"></div>
  <div class="swatch" style="background:#27363d" title="tool result block"></div>
</div>
</div>

<!-- monokai -->
<div class="theme-card">

### monokai

Monokai color scheme — `thinkt theme set monokai`

<div class="palette-row-label">chrome &amp; text</div>
<div class="palette">
  <div class="swatch" style="background:#9d65ff" title="accent"></div>
  <div class="swatch" style="background:#9d65ff" title="border active"></div>
  <div class="swatch" style="background:#625e4c" title="border inactive"></div>
  <div class="swatch" style="background:#c4c5b5" title="text primary"></div>
  <div class="swatch" style="background:#625e4c" title="text secondary"></div>
  <div class="swatch" style="background:#454338" title="text muted"></div>
</div>
<div class="palette-row-label">labels</div>
<div class="palette-labels">
  <div class="swatch" style="background:#9d65ff" title="user label"></div>
  <div class="swatch" style="background:#98e024" title="assistant label"></div>
  <div class="swatch" style="background:#f4005f" title="thinking label"></div>
  <div class="swatch" style="background:#fa8419" title="tool label"></div>
  <div class="swatch" style="background:#9d65ff" title="confirm selected"></div>
</div>
<div class="palette-row-label">block backgrounds</div>
<div class="palette-blocks">
  <div class="swatch" style="background:#2e253c" title="user block"></div>
  <div class="swatch" style="background:#2d381c" title="assistant block"></div>
  <div class="swatch" style="background:#341722" title="thinking block"></div>
  <div class="swatch" style="background:#35271a" title="tool call block"></div>
  <div class="swatch" style="background:#213033" title="tool result block"></div>
</div>
</div>

---

## Importing iTerm2 Color Schemes

You can import any `.itermcolors` file from the [iTerm2-Color-Schemes](https://github.com/mbadolato/iTerm2-Color-Schemes) repository (450+ schemes) or any other source:

```bash
# Import a downloaded .itermcolors file
thinkt theme import ~/Downloads/Zenburn.itermcolors

# Import with a custom name
thinkt theme import scheme.itermcolors --name my-zenburn

# Activate the imported theme
thinkt theme set my-zenburn
```

The importer maps iTerm2 ANSI colors to thinkt's semantic theme fields:

| thinkt element | iTerm2 source |
|----------------|---------------|
| Accent / active border | Ansi 12 (bright blue) |
| Inactive border | Ansi 8 (bright black) |
| Text primary | Foreground |
| Text secondary | Ansi 8 |
| User label | Ansi 4 (blue) |
| Assistant label | Ansi 2 (green) |
| Thinking label | Ansi 5 (magenta) |
| Tool label | Ansi 3 (yellow) |
| Block backgrounds | Blended from Background + ANSI accent |

Imported themes are saved to `~/.thinkt/themes/` and can be further customized with `thinkt theme builder`.

---

## Creating Custom Themes

Beyond importing, you can create themes from scratch:

```bash
# Launch the interactive theme builder
thinkt theme builder my-theme

# Or start from an existing theme
thinkt theme builder dracula
```

Theme JSON files are stored in `~/.thinkt/themes/` and follow the structure defined in the [theme command reference](/command/thinkt_theme).

---

## Regenerating Built-in Themes

The built-in themes are generated from iTerm2 color schemes using a code generator:

```bash
go run cmd/gen-themes/main.go
```

This downloads the curated set from the [iTerm2-Color-Schemes](https://github.com/mbadolato/iTerm2-Color-Schemes) repository and writes JSON files to `internal/tui/theme/themes/`. These are embedded into the binary at build time via `go:embed`.

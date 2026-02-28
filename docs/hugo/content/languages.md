---
title: "Languages"
weight: 7
---

# Languages

thinkt supports internationalization (i18n). You can switch between different languages for the CLI, TUI, and server messages.

## Current Languages

We currently support:
- **English** (`en`) — Default
- **Chinese (Simplified)** (`zh-Hans`) — 简体中文
- **Spanish** (`es`) — Español

## How to Set Language

### 1. Environment Variable
The easiest way to quickly switch languages is via the `THINKT_LANG` environment variable:

```bash
# Run in Spanish
THINKT_LANG=es thinkt

# Run in Chinese
THINKT_LANG=zh-Hans thinkt
```

### 2. Configuration
You can permanently set your preferred language via the CLI:

```bash
# List available languages and coverage
thinkt language list

# Interactively choose a language
thinkt language set

# Set directly by tag
thinkt language set es
```

### 3. System Locale
By default, thinkt will try to detect your system language using the `LANG` or `LC_ALL` environment variables.

## Adding a New Language

We welcome contributions for new languages!

### 1. Create a Translation File
Create a new TOML file in `internal/i18n/locales/` named with the BCP 47 language tag (e.g., `fr.toml` for French).

### 2. Translate the Messages
Use `internal/i18n/locales/en.toml` or `zh-Hans.toml` as a template for message IDs.

```toml
[common.loading]
other = "Chargement..."
```

### 3. Guidelines
- Ensure all `other` strings are on a single line. If you need a newline, use `\n`.
- Use `{{.Count}}` for plural counts.
- Keep message IDs exactly as they appear in the template.

### 4. Verification
You can run the coverage test to see which strings are missing:

```bash
go test -v ./internal/i18n/ -run TestTranslationCoverage
```

And ensure your syntax is correct:

```bash
go test -v ./internal/i18n/ -run TestLocaleSyntax
```

For more detailed technical information on the i18n implementation, see `docs/I18N.md` in the repository.

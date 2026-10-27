/** @type {import("tailwindcss").Config} */
module.exports = {
  // The kit's HTML lives in the Go module, so Tailwind never sees it. Every kit
  // component this app renders is mirrored in web/templates/components/ to get
  // its classes compiled (amarra-cais#372). Add an override before adopting a
  // new <.tag> component.
  content: ["./web/templates/**/*.html"],
  safelist: [
    "cais-password-wrap",
    "cais-password-toggle",
    "cais-chat-scroll-down",
    "cais-thinking",
    "cais-thinking-dots",
    "cais-select-search",
    "cais-select-search-native",
    "cais-select-search-trigger",
    "cais-select-search-panel",
    "cais-select-search-input",
    "cais-select-search-list",
    "cais-select-search-option",
    "cais-select-search-label",
    "cais-select-search-chevron",
    "is-selected",
    "is-highlighted",
    "is-hidden",
  ],
  theme: {
    extend: {
      colors: {
        /* Kit components bind to these five names through CSS variables
           (input.css), so the amarra kit keeps working while the app uses the
           role tokens below (#258). */
        ink: "rgb(var(--amarra-ink) / <alpha-value>)",
        foam: "rgb(var(--amarra-foam) / <alpha-value>)",
        copper: "rgb(var(--amarra-copper) / <alpha-value>)",
        tide: "rgb(var(--amarra-tide) / <alpha-value>)",
        coral: "rgb(var(--amarra-coral) / <alpha-value>)",

        /* Layers (DESIGN.md "Elevation & Depth"): canvas → surface → elevated. */
        canvas: "#FAF9F6",
        surface: "#F4F3EF",
        elevated: "#FFFFFF",
        "container-low": "#F4F3F1",
        container: "#EFEEEB",
        "container-high": "#E9E8E5",
        "container-highest": "#E3E2E0",

        /* Text: ink for prose and numbers, muted for metadata, outline for 1px
           strokes that are not borders. */
        muted: "#64625D",
        outline: "#727974",
        "outline-variant": "#C2C8C2",

        /* Primary — actions, confirmed commitments, active selection, brand. */
        pine: "#1B382B",
        "pine-deep": "#042217",
        "pine-hover": "#142B21",
        "pine-muted": "#82A291",
        moss: "#C9EAD7",
        "moss-dim": "#ADCEBC",

        /* Secondary — functional controls, navigational links, high-contrast. */
        slate: "#2C2C2C",
        "slate-soft": "#5F5E5E",
        "slate-tint": "#E4E2E1",

        /* Tertiary — destructive, cancellation, unavailable constraint. */
        clay: "#C26343",
        "clay-soft": "#E57E5C",
        "clay-tint": "#FFDBD0",

        /* Structure */
        line: "#E5E3DC",
        "line-strong": "#D4D1C7",
      },
      fontFamily: {
        sans: [
          "Hanken Grotesk",
          "Avenir Next",
          "Segoe UI",
          "Helvetica Neue",
          "system-ui",
          "sans-serif",
        ],
        /* Headings stay Grotesk in this system; the alias keeps the kit and the
           scaffold's font-serif call sites consistent. */
        serif: [
          "Hanken Grotesk",
          "Avenir Next",
          "Segoe UI",
          "Helvetica Neue",
          "system-ui",
          "sans-serif",
        ],
        display: [
          "Hanken Grotesk",
          "Avenir Next",
          "Segoe UI",
          "Helvetica Neue",
          "system-ui",
          "sans-serif",
        ],
        mono: ["JetBrains Mono", "ui-monospace", "SFMono-Regular", "Menlo", "monospace"],
      },
      fontSize: {
        "headline-xl": [
          "2.5rem",
          { lineHeight: "3rem", letterSpacing: "-0.03em", fontWeight: "600" },
        ],
        "headline-lg": [
          "1.75rem",
          { lineHeight: "2.25rem", letterSpacing: "-0.02em", fontWeight: "600" },
        ],
        "headline-md": [
          "1.375rem",
          { lineHeight: "1.875rem", letterSpacing: "-0.015em", fontWeight: "500" },
        ],
        "headline-sm": [
          "1.125rem",
          { lineHeight: "1.625rem", letterSpacing: "-0.01em", fontWeight: "500" },
        ],
        "body-lg": ["1.0625rem", { lineHeight: "1.625rem" }],
        "body-md": ["0.9375rem", { lineHeight: "1.4375rem" }],
        "body-sm": ["0.8125rem", { lineHeight: "1.1875rem" }],
        "label-ui": ["0.75rem", { lineHeight: "1rem", letterSpacing: "0.04em", fontWeight: "600" }],
        "label-mono": ["0.75rem", { lineHeight: "1rem", letterSpacing: "0.01em" }],
        "label-mono-lg": ["0.875rem", { lineHeight: "1.25rem", letterSpacing: "-0.02em" }],
      },
      borderRadius: {
        DEFAULT: "0.5rem",
        md: "0.75rem",
        lg: "1rem",
        xl: "1.5rem",
      },
      spacing: {
        "space-xs": "0.25rem",
        "space-sm": "0.5rem",
        "space-md": "1rem",
        "space-lg": "1.5rem",
        "space-xl": "2.5rem",
        margin: "1.25rem",
      },
      boxShadow: {
        "2xs": "0 1px 2px 0 rgb(0 0 0 / 0.04)",
        xs: "0 1px 2px 0 rgb(0 0 0 / 0.04)",
        layer: "0 12px 32px -8px rgba(26, 26, 26, 0.05)",
      },
    },
  },
  plugins: [],
};

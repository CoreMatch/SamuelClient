// Shared visual tokens, aligned with the MatchCoreArena client palette
// (competitive-ui-research 04 §A7): spacing 4/8/12/16/24, radii 8/12, type
// scale 12/14/16/20/28, dark baseline with a light variant kept, and one
// semantic color per meaning. No ad-hoc hex codes outside this table —
// views reference these tokens (directly or through the MUI theme).

import { createTheme, Theme } from '@mui/material/styles';

export const tokens = {
  // Allowed spacing steps (px). With MUI `spacing: 4`, sx indices map to
  // 1→4, 2→8, 3→12, 4→16, 6→24.
  spacing: [4, 8, 12, 16, 24],
  radius: { md: 8, lg: 12 },
  fontSize: { xs: 12, sm: 14, md: 16, lg: 20, xl: 28 },
  color: {
    // Brand accent: primary CTAs and rank changes only.
    accent: '#e94560',
    accentDark: '#c81e45',
    // Dark baseline surfaces.
    deep: '#1a1a2e',
    base: '#16213e',
    card: '#0f3460',
    // Semantic — exactly one per meaning.
    success: '#66bb6a',
    warning: '#ffcc80',
    danger: '#ff6b6b',
    info: '#8ecdf5',
    // Text on dark surfaces (alpha variants of white, not new hues).
    text: 'rgba(255, 255, 255, 0.87)',
    textMuted: 'rgba(255, 255, 255, 0.6)',
    divider: 'rgba(255, 255, 255, 0.12)',
  },
} as const;

// buildTheme returns the MUI theme for a mode. Dark is the baseline; light
// is kept as a variant using the same accent/semantic colors.
export function buildTheme(mode: 'dark' | 'light' = 'dark'): Theme {
  const dark = mode === 'dark';
  return createTheme({
    palette: {
      mode,
      primary: { main: tokens.color.accent, dark: tokens.color.accentDark },
      success: { main: tokens.color.success },
      warning: { main: tokens.color.warning },
      error: { main: tokens.color.danger },
      info: { main: tokens.color.info },
      background: dark
        ? { default: tokens.color.base, paper: tokens.color.card }
        : { default: '#ffffff', paper: '#ffffff' },
      text: dark
        ? { primary: tokens.color.text, secondary: tokens.color.textMuted }
        : undefined,
      divider: dark ? tokens.color.divider : 'rgba(0, 0, 0, 0.12)',
    },
    shape: { borderRadius: tokens.radius.md },
    spacing: 4,
    typography: {
      fontFamily:
        "Inter, system-ui, -apple-system, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif",
      caption: { fontSize: tokens.fontSize.xs },
      body2: { fontSize: tokens.fontSize.sm },
      body1: { fontSize: tokens.fontSize.md },
      h6: { fontSize: tokens.fontSize.lg },
      h4: { fontSize: tokens.fontSize.xl },
    },
    components: {
      MuiPaper: {
        styleOverrides: { root: { backgroundImage: 'none' } },
      },
      MuiButton: {
        styleOverrides: { root: { borderRadius: tokens.radius.md, textTransform: 'none' } },
      },
    },
  });
}

export const darkTheme = buildTheme('dark');
export const lightTheme = buildTheme('light');

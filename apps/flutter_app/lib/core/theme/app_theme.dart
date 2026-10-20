import 'package:flutter/material.dart';

import 'tokens.dart';

/// Font families. Fraunces (display), Atkinson Hyperlegible (body/UI) and
/// IBM Plex Mono (coordinates, ids, counts) are OFL fonts meant to be bundled
/// under `assets/fonts/` (see docs/architecture/flutter.md). Until the files
/// are added the platform fallbacks below are used, so nothing needs the
/// network at runtime.
const String kDisplayFont = 'Fraunces';
const String kBodyFont = 'AtkinsonHyperlegible';
const String kMonoFont = 'IBMPlexMono';

const List<String> kBodyFallback = [
  'Atkinson Hyperlegible',
  'Segoe UI',
  'Noto Sans',
  'Ubuntu',
  'Cantarell',
  'DejaVu Sans',
  'Arial',
];
const List<String> kDisplayFallback = [
  'Fraunces',
  'Georgia',
  'Noto Serif',
  'DejaVu Serif',
  'Times New Roman',
];
const List<String> kMonoFallback = [
  'IBM Plex Mono',
  'Noto Sans Mono',
  'DejaVu Sans Mono',
  'Consolas',
  'Courier New',
];

/// Monospace style for coordinates, reference numbers (ET-2026-000123) and
/// counts.
TextStyle monoStyle([TextStyle? base]) => (base ?? const TextStyle()).copyWith(
      fontFamily: kMonoFont,
      fontFamilyFallback: kMonoFallback,
      fontFeatures: const [FontFeature.tabularFigures()],
    );

class AppTheme {
  const AppTheme._();

  static ThemeData dark() => _build(EcoColors.dark, Brightness.dark);
  static ThemeData light() => _build(EcoColors.light, Brightness.light);
  static ThemeData highContrast() =>
      _build(EcoColors.highContrast, Brightness.light);

  static ThemeData _build(EcoColors c, Brightness brightness) {
    final scheme = ColorScheme(
      brightness: brightness,
      primary: c.green,
      onPrimary: c.isDark ? const Color(0xFF04210F) : Colors.white,
      secondary: c.teal,
      onSecondary: c.isDark ? const Color(0xFF02201C) : Colors.white,
      error: c.critical,
      onError: c.isDark ? const Color(0xFF2B0A09) : Colors.white,
      surface: c.surface,
      onSurface: c.text,
      onSurfaceVariant: c.textMuted,
      surfaceContainerHighest: c.surfaceAlt,
      outline: c.border,
      outlineVariant: c.border,
    );

    final base = ThemeData(
      useMaterial3: true,
      brightness: brightness,
      colorScheme: scheme,
      scaffoldBackgroundColor: c.bg,
      fontFamily: kBodyFont,
      fontFamilyFallback: kBodyFallback,
      visualDensity: VisualDensity.standard,
    );
    TextStyle? display(TextStyle? t) => t?.copyWith(
          fontFamily: kDisplayFont,
          fontFamilyFallback: kDisplayFallback,
          fontWeight: FontWeight.w600,
        );
    final tt = base.textTheme.apply(bodyColor: c.text, displayColor: c.text);
    // Body text >= 16sp; small text never below 13sp.
    final textTheme = tt.copyWith(
      displayLarge: display(tt.displayLarge),
      displayMedium: display(tt.displayMedium),
      displaySmall: display(tt.displaySmall),
      headlineLarge: display(tt.headlineLarge),
      headlineMedium: display(tt.headlineMedium),
      headlineSmall: display(tt.headlineSmall),
      titleLarge: display(tt.titleLarge),
      titleMedium: tt.titleMedium?.copyWith(fontSize: 17, fontWeight: FontWeight.w700),
      titleSmall: tt.titleSmall?.copyWith(fontSize: 15, fontWeight: FontWeight.w700),
      bodyLarge: tt.bodyLarge?.copyWith(fontSize: 17),
      bodyMedium: tt.bodyMedium?.copyWith(fontSize: 16),
      bodySmall: tt.bodySmall?.copyWith(fontSize: 14),
      labelLarge: tt.labelLarge?.copyWith(fontSize: 15),
      labelMedium: tt.labelMedium?.copyWith(fontSize: 14),
      labelSmall: tt.labelSmall?.copyWith(fontSize: 13),
    );

    const radius = BorderRadius.all(Radius.circular(Dimens.radius));
    final border = OutlineInputBorder(
      borderRadius: radius,
      borderSide: BorderSide(color: c.border),
    );

    return base.copyWith(
      extensions: [c],
      textTheme: textTheme,
      dividerTheme: DividerThemeData(color: c.border, space: 1, thickness: 1),
      appBarTheme: AppBarTheme(
        backgroundColor: c.surface,
        foregroundColor: c.text,
        elevation: 0,
        scrolledUnderElevation: 0,
      ),
      cardTheme: CardThemeData(
        color: c.surface,
        elevation: 0,
        margin: EdgeInsets.zero,
        shape: RoundedRectangleBorder(
          borderRadius: radius,
          side: BorderSide(color: c.border),
        ),
      ),
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: c.surfaceAlt.withValues(alpha: c.isDark ? 0.5 : 0.7),
        border: border,
        enabledBorder: border,
        focusedBorder: border.copyWith(
          borderSide: BorderSide(color: c.teal, width: 1.5),
        ),
        errorBorder: border.copyWith(
          borderSide: BorderSide(color: c.critical),
        ),
        focusedErrorBorder: border.copyWith(
          borderSide: BorderSide(color: c.critical, width: 1.5),
        ),
        contentPadding: const EdgeInsets.symmetric(
          horizontal: Dimens.md,
          vertical: Dimens.md,
        ),
        hintStyle: TextStyle(color: c.textMuted),
        isDense: true,
      ),
      filledButtonTheme: FilledButtonThemeData(
        style: FilledButton.styleFrom(
          minimumSize: const Size(0, 48),
          shape: const RoundedRectangleBorder(borderRadius: radius),
        ),
      ),
      outlinedButtonTheme: OutlinedButtonThemeData(
        style: OutlinedButton.styleFrom(
          minimumSize: const Size(0, 48),
          foregroundColor: c.text,
          side: BorderSide(color: c.border),
          shape: const RoundedRectangleBorder(borderRadius: radius),
        ),
      ),
      textButtonTheme: TextButtonThemeData(
        style: TextButton.styleFrom(
          foregroundColor: c.teal,
          minimumSize: const Size(48, 48),
        ),
      ),
      focusColor: c.teal.withValues(alpha: 0.2),
      iconButtonTheme: IconButtonThemeData(
        style: IconButton.styleFrom(minimumSize: const Size(48, 48)),
      ),
      chipTheme: ChipThemeData(
        backgroundColor: Colors.transparent,
        selectedColor: c.teal.withValues(alpha: 0.2),
        side: BorderSide(color: c.border),
        labelStyle: TextStyle(color: c.text, fontSize: 14),
        shape: const StadiumBorder(),
        showCheckmark: false,
      ),
      dialogTheme: DialogThemeData(
        backgroundColor: c.surface,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(Dimens.radiusLg),
          side: BorderSide(color: c.border),
        ),
      ),
      snackBarTheme: SnackBarThemeData(
        behavior: SnackBarBehavior.floating,
        backgroundColor: c.surfaceAlt,
        contentTextStyle: TextStyle(color: c.text),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(10)),
      ),
      navigationBarTheme: NavigationBarThemeData(
        backgroundColor: c.surface,
        indicatorColor: c.teal.withValues(alpha: 0.2),
        labelTextStyle: WidgetStatePropertyAll(
          TextStyle(fontSize: 13, color: c.text),
        ),
      ),
      tabBarTheme: TabBarThemeData(
        labelColor: c.teal,
        unselectedLabelColor: c.textMuted,
        indicatorColor: c.teal,
        dividerColor: c.border,
      ),
      popupMenuTheme: PopupMenuThemeData(
        color: c.surface,
        shape: RoundedRectangleBorder(
          borderRadius: radius,
          side: BorderSide(color: c.border),
        ),
      ),
      switchTheme: SwitchThemeData(
        thumbColor: WidgetStateProperty.resolveWith(
          (s) => s.contains(WidgetState.selected) ? c.teal : c.textMuted,
        ),
        trackColor: WidgetStateProperty.resolveWith(
          (s) => s.contains(WidgetState.selected)
              ? c.teal.withValues(alpha: 0.35)
              : c.border,
        ),
      ),
    );
  }
}

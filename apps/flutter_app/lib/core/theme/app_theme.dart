import 'package:flutter/material.dart';

import 'tokens.dart';

/// Offline-safe font stack: no network font loading. The first family found
/// on the platform wins (Roboto on Android, Segoe UI on Windows, SF on Apple,
/// Noto/Ubuntu/DejaVu on Linux).
const List<String> kFontFallback = [
  'Inter',
  'Roboto',
  'Segoe UI',
  'Noto Sans',
  'Ubuntu',
  'Cantarell',
  'DejaVu Sans',
  'Arial',
];

class AppTheme {
  const AppTheme._();

  static ThemeData dark() => _build(EcoColors.dark, Brightness.dark);
  static ThemeData light() => _build(EcoColors.light, Brightness.light);

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
      fontFamilyFallback: kFontFallback,
      visualDensity: VisualDensity.adaptivePlatformDensity,
    );
    final textTheme = base.textTheme.apply(
      bodyColor: c.text,
      displayColor: c.text,
      fontFamilyFallback: kFontFallback,
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
          minimumSize: const Size(0, 44),
          shape: const RoundedRectangleBorder(borderRadius: radius),
        ),
      ),
      outlinedButtonTheme: OutlinedButtonThemeData(
        style: OutlinedButton.styleFrom(
          minimumSize: const Size(0, 44),
          foregroundColor: c.text,
          side: BorderSide(color: c.border),
          shape: const RoundedRectangleBorder(borderRadius: radius),
        ),
      ),
      textButtonTheme: TextButtonThemeData(
        style: TextButton.styleFrom(foregroundColor: c.teal),
      ),
      chipTheme: ChipThemeData(
        backgroundColor: Colors.transparent,
        selectedColor: c.teal.withValues(alpha: 0.2),
        side: BorderSide(color: c.border),
        labelStyle: TextStyle(color: c.text, fontSize: 13),
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
          TextStyle(fontSize: 12, color: c.text),
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

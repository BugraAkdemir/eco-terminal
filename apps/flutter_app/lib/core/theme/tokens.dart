import 'package:flutter/material.dart';

import '../models/enums.dart';

/// Spacing / radius / breakpoint constants (spec §18).
class Dimens {
  const Dimens._();

  static const double xs = 4;
  static const double sm = 8;
  static const double md = 12;
  static const double lg = 16;
  static const double xl = 24;
  static const double xxl = 32;

  static const double radius = 12;
  static const double radiusSm = 8;
  static const double radiusLg = 16;

  /// <700 mobile (bottom nav), 700-1100 tablet, >1100 desktop.
  static const double mobileMax = 700;
  static const double desktopMin = 1100;

  static const double headerHeight = 64;
}

enum ScreenSize { mobile, tablet, desktop }

ScreenSize screenSizeForWidth(double width) {
  if (width < Dimens.mobileMax) return ScreenSize.mobile;
  if (width <= Dimens.desktopMin) return ScreenSize.tablet;
  return ScreenSize.desktop;
}

/// Eco Terminal colour tokens as a [ThemeExtension]: `context.eco`.
@immutable
class EcoColors extends ThemeExtension<EcoColors> {
  const EcoColors({
    required this.bg,
    required this.surface,
    required this.surfaceAlt,
    required this.sidebar,
    required this.green,
    required this.teal,
    required this.warning,
    required this.critical,
    required this.info,
    required this.text,
    required this.textMuted,
    required this.border,
    required this.glass,
    required this.isDark,
  });

  final Color bg;
  final Color surface;
  final Color surfaceAlt;
  final Color sidebar;
  final Color green;
  final Color teal;
  final Color warning;
  final Color critical;
  final Color info;
  final Color text;
  final Color textMuted;
  final Color border;

  /// Translucent surface used for glass panels (sparingly).
  final Color glass;
  final bool isDark;

  static const dark = EcoColors(
    bg: Color(0xFF07131E),
    surface: Color(0xFF0D2232),
    surfaceAlt: Color(0xFF123047),
    sidebar: Color(0xFF092B45),
    green: Color(0xFF36C879),
    teal: Color(0xFF22B8A8),
    warning: Color(0xFFF1AA4C),
    critical: Color(0xFFE66B68),
    info: Color(0xFF5AA9E6),
    text: Color(0xFFF2F7F7),
    textMuted: Color(0xFFA9BCC5),
    border: Color(0x33A9BCC5),
    glass: Color(0xB30D2232),
    isDark: true,
  );

  static const light = EcoColors(
    bg: Color(0xFFF2F7F8),
    surface: Color(0xFFFFFFFF),
    surfaceAlt: Color(0xFFE8F0F3),
    sidebar: Color(0xFFE3EDF2),
    green: Color(0xFF1C9B57),
    teal: Color(0xFF128F82),
    warning: Color(0xFFB9731A),
    critical: Color(0xFFC8443F),
    info: Color(0xFF2B7BBD),
    text: Color(0xFF0B1C27),
    textMuted: Color(0xFF506672),
    border: Color(0x330B1C27),
    glass: Color(0xCCFFFFFF),
    isDark: false,
  );

  Color statusColor(ObservationStatus s) {
    switch (s) {
      case ObservationStatus.draft:
        return isDark ? const Color(0xFF8A9BA6) : const Color(0xFF6B7C86);
      case ObservationStatus.submitted:
        return info;
      case ObservationStatus.aiProcessed:
        return isDark ? const Color(0xFFA78BFA) : const Color(0xFF7C5CD6);
      case ObservationStatus.underReview:
        return warning;
      case ObservationStatus.verified:
        return green;
      case ObservationStatus.needsInformation:
        return isDark ? const Color(0xFFF28CB1) : const Color(0xFFC2457C);
      case ObservationStatus.rejected:
        return critical;
      case ObservationStatus.resolved:
        return teal;
      case ObservationStatus.unknown:
        return textMuted;
    }
  }

  Color categoryColor(ObservationCategory c) {
    switch (c) {
      case ObservationCategory.animal:
        return const Color(0xFFE8A94A);
      case ObservationCategory.plant:
        return const Color(0xFF5CCB6E);
      case ObservationCategory.water:
        return const Color(0xFF4DA3E8);
      case ObservationCategory.forest:
        return const Color(0xFF2E9E6B);
      case ObservationCategory.pollution:
        return const Color(0xFFC97B4A);
      case ObservationCategory.injuredWildlife:
        return critical;
      case ObservationCategory.other:
        return const Color(0xFF9AA9B2);
    }
  }

  @override
  EcoColors copyWith({Color? bg, Color? surface}) => EcoColors(
        bg: bg ?? this.bg,
        surface: surface ?? this.surface,
        surfaceAlt: surfaceAlt,
        sidebar: sidebar,
        green: green,
        teal: teal,
        warning: warning,
        critical: critical,
        info: info,
        text: text,
        textMuted: textMuted,
        border: border,
        glass: glass,
        isDark: isDark,
      );

  @override
  EcoColors lerp(ThemeExtension<EcoColors>? other, double t) {
    if (other is! EcoColors) return this;
    Color l(Color a, Color b) => Color.lerp(a, b, t)!;
    return EcoColors(
      bg: l(bg, other.bg),
      surface: l(surface, other.surface),
      surfaceAlt: l(surfaceAlt, other.surfaceAlt),
      sidebar: l(sidebar, other.sidebar),
      green: l(green, other.green),
      teal: l(teal, other.teal),
      warning: l(warning, other.warning),
      critical: l(critical, other.critical),
      info: l(info, other.info),
      text: l(text, other.text),
      textMuted: l(textMuted, other.textMuted),
      border: l(border, other.border),
      glass: l(glass, other.glass),
      isDark: t < 0.5 ? isDark : other.isDark,
    );
  }
}

extension EcoThemeContext on BuildContext {
  EcoColors get eco =>
      Theme.of(this).extension<EcoColors>() ??
      (Theme.of(this).brightness == Brightness.dark
          ? EcoColors.dark
          : EcoColors.light);
}

/// Icon per category (shared by map markers and lists).
IconData categoryIcon(ObservationCategory c) {
  switch (c) {
    case ObservationCategory.animal:
      return Icons.pets;
    case ObservationCategory.plant:
      return Icons.eco;
    case ObservationCategory.water:
      return Icons.water_drop;
    case ObservationCategory.forest:
      return Icons.park;
    case ObservationCategory.pollution:
      return Icons.warning_amber_rounded;
    case ObservationCategory.injuredWildlife:
      return Icons.healing;
    case ObservationCategory.other:
      return Icons.place;
  }
}

/// Icon per status so status is never conveyed by colour alone.
IconData statusIcon(ObservationStatus s) {
  switch (s) {
    case ObservationStatus.draft:
      return Icons.edit_note;
    case ObservationStatus.submitted:
      return Icons.send;
    case ObservationStatus.aiProcessed:
      return Icons.auto_awesome;
    case ObservationStatus.underReview:
      return Icons.hourglass_top;
    case ObservationStatus.verified:
      return Icons.verified;
    case ObservationStatus.needsInformation:
      return Icons.help_outline;
    case ObservationStatus.rejected:
      return Icons.cancel_outlined;
    case ObservationStatus.resolved:
      return Icons.task_alt;
    case ObservationStatus.unknown:
      return Icons.help;
  }
}

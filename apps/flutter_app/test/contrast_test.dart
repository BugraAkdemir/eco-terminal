import 'dart:math' as math;

import 'package:eco_terminal/core/models/enums.dart';
import 'package:eco_terminal/core/theme/tokens.dart';
import 'package:flutter/painting.dart';
import 'package:flutter_test/flutter_test.dart';

double _lum(Color c) {
  double ch(double v) =>
      v <= 0.03928 ? v / 12.92 : math.pow((v + 0.055) / 1.055, 2.4).toDouble();
  return 0.2126 * ch(c.r) + 0.7152 * ch(c.g) + 0.0722 * ch(c.b);
}

double contrast(Color a, Color b) {
  final la = _lum(a), lb = _lum(b);
  return (math.max(la, lb) + 0.05) / (math.min(la, lb) + 0.05);
}

void main() {
  const themes = {
    'dark': EcoColors.dark,
    'light': EcoColors.light,
    'highContrast': EcoColors.highContrast,
  };

  for (final entry in themes.entries) {
    test('${entry.key}: text and accent colours reach WCAG AA (4.5:1)', () {
      final eco = entry.value;
      final fg = <String, Color>{
        'text': eco.text,
        'textMuted': eco.textMuted,
        'green': eco.green,
        'teal': eco.teal,
        'warning': eco.warning,
        'critical': eco.critical,
        'info': eco.info,
        for (final s in ObservationStatus.lifecycle)
          'status ${s.name}': eco.statusColor(s),
      };
      for (final bg in [eco.bg, eco.surface, eco.sidebar]) {
        for (final f in fg.entries) {
          expect(
            contrast(f.value, bg),
            greaterThanOrEqualTo(4.5),
            reason: '${entry.key}: ${f.key} on $bg',
          );
        }
      }
    });
  }

  test('body text reaches 7:1 in every theme', () {
    for (final eco in themes.values) {
      expect(contrast(eco.text, eco.bg), greaterThanOrEqualTo(7));
      expect(contrast(eco.text, eco.surface), greaterThanOrEqualTo(7));
    }
    expect(contrast(EcoColors.highContrast.textMuted, EcoColors.highContrast.bg),
        greaterThanOrEqualTo(7));
  });
}

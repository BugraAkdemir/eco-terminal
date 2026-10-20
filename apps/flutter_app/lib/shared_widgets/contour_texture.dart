import 'dart:math' as math;

import 'package:flutter/material.dart';

import '../core/theme/tokens.dart';

/// Faint topographic contour lines ("Saha Atlası"). Static, ~5% opacity,
/// drawn once per size; never animated.
class ContourPainter extends CustomPainter {
  const ContourPainter({required this.color, this.opacity = 0.05, this.seed = 7});

  final Color color;
  final double opacity;
  final int seed;

  @override
  void paint(Canvas canvas, Size size) {
    if (size.isEmpty) return;
    final paint = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = 1
      ..color = color.withValues(alpha: opacity);
    final cx = size.width * (0.25 + 0.1 * (seed % 5));
    final cy = size.height * 0.75;
    final maxR = math.sqrt(size.width * size.width + size.height * size.height);
    for (var r = 28.0; r < maxR; r += 26) {
      final path = Path();
      const steps = 72;
      for (var i = 0; i <= steps; i++) {
        final a = 2 * math.pi * i / steps;
        // Deterministic wobble so rings look like terrain, not circles.
        final wobble = 1 +
            0.10 * math.sin(3 * a + r / 45 + seed) +
            0.05 * math.sin(5 * a - r / 30);
        final x = cx + r * wobble * math.cos(a) * 1.25;
        final y = cy + r * wobble * math.sin(a);
        i == 0 ? path.moveTo(x, y) : path.lineTo(x, y);
      }
      path.close();
      canvas.drawPath(path, paint);
    }
  }

  @override
  bool shouldRepaint(ContourPainter old) =>
      old.color != color || old.opacity != opacity || old.seed != seed;
}

/// Paints [child] over the contour texture (clipped to its bounds).
class ContourBackground extends StatelessWidget {
  const ContourBackground({
    super.key,
    required this.child,
    this.opacity = 0.05,
    this.seed = 7,
  });

  final Widget child;
  final double opacity;
  final int seed;

  @override
  Widget build(BuildContext context) {
    final eco = context.eco;
    return ClipRect(
      child: CustomPaint(
        painter: ContourPainter(color: eco.text, opacity: opacity, seed: seed),
        child: child,
      ),
    );
  }
}

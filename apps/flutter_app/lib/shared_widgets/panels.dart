import 'dart:ui';

import 'package:flutter/material.dart';

import '../core/theme/tokens.dart';

/// Solid surface with a thin border: the default container.
class SolidPanel extends StatelessWidget {
  const SolidPanel({
    super.key,
    required this.child,
    this.padding = const EdgeInsets.all(Dimens.lg),
    this.color,
    this.radius = Dimens.radius,
    this.borderColor,
  });

  final Widget child;
  final EdgeInsetsGeometry padding;
  final Color? color;
  final double radius;
  final Color? borderColor;

  @override
  Widget build(BuildContext context) {
    final eco = context.eco;
    return DecoratedBox(
      decoration: BoxDecoration(
        color: color ?? eco.surface,
        borderRadius: BorderRadius.circular(radius),
        border: Border.all(color: borderColor ?? eco.border),
      ),
      child: Padding(padding: padding, child: child),
    );
  }
}

/// Subtle translucent, blurred surface. Use sparingly (floating map chrome),
/// never for dense data: readable data sits on [SolidPanel].
class GlassPanel extends StatelessWidget {
  const GlassPanel({
    super.key,
    required this.child,
    this.padding = const EdgeInsets.all(Dimens.md),
    this.radius = Dimens.radius,
    this.blur = 12,
  });

  final Widget child;
  final EdgeInsetsGeometry padding;
  final double radius;
  final double blur;

  @override
  Widget build(BuildContext context) {
    final eco = context.eco;
    return ClipRRect(
      borderRadius: BorderRadius.circular(radius),
      child: BackdropFilter(
        filter: ImageFilter.blur(sigmaX: blur, sigmaY: blur),
        child: DecoratedBox(
          decoration: BoxDecoration(
            color: eco.glass,
            borderRadius: BorderRadius.circular(radius),
            border: Border.all(color: eco.border),
          ),
          child: Padding(padding: padding, child: child),
        ),
      ),
    );
  }
}

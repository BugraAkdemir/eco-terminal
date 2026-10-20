import 'dart:math' as math;

import 'package:flutter/material.dart';

/// Short fade + rise reveal for list items (150-250 ms). Items further down
/// the list take slightly longer, which reads as a stagger without timers.
/// Skipped entirely when the OS asks to reduce motion.
class Reveal extends StatelessWidget {
  const Reveal({super.key, required this.child, this.index = 0});

  final Widget child;
  final int index;

  @override
  Widget build(BuildContext context) {
    if (MediaQuery.maybeDisableAnimationsOf(context) ?? false) return child;
    final ms = 150 + 25 * math.min(index, 4).toInt();
    return TweenAnimationBuilder<double>(
      tween: Tween(begin: 0, end: 1),
      duration: Duration(milliseconds: ms),
      curve: Curves.easeOut,
      builder: (context, t, child) => Opacity(
        opacity: t,
        child: Transform.translate(offset: Offset(0, (1 - t) * 8), child: child),
      ),
      child: child,
    );
  }
}

/// Bottom action bar that stays visible above the keyboard / gesture area.
/// Put the single primary action of a mobile screen in it (>= 48dp high).
class StickyBottomBar extends StatelessWidget {
  const StickyBottomBar({super.key, required this.child});
  final Widget child;

  @override
  Widget build(BuildContext context) {
    final cs = Theme.of(context).colorScheme;
    return Material(
      color: cs.surface,
      elevation: 8,
      child: SafeArea(
        top: false,
        child: Padding(
          padding: const EdgeInsets.fromLTRB(16, 12, 16, 12),
          child: child,
        ),
      ),
    );
  }
}

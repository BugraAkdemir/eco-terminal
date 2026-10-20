import 'package:flutter/material.dart';

import '../../../core/l10n/strings.dart';
import '../../../core/models/models.dart';
import '../../../core/theme/tokens.dart';

/// Map pin. Category = SHAPE + icon (+ colour); status = thick ring colour
/// (and a fuller fill once an expert verified the record).
class ObservationMarkerWidget extends StatelessWidget {
  const ObservationMarkerWidget({super.key, required this.observation});

  final Observation observation;

  @override
  Widget build(BuildContext context) {
    final eco = context.eco;
    final s = context.s;
    final cat = observation.category;
    final fill = eco.categoryColor(cat);
    final ring = eco.statusColor(observation.status);
    final verified = observation.status == ObservationStatus.verified ||
        observation.status == ObservationStatus.resolved;
    return Semantics(
      label:
          '${s.categoryLabel(cat)}: ${observation.displayTitle}, ${s.statusLabel(observation.status)}',
      button: true,
      excludeSemantics: true,
      child: Container(
        decoration: ShapeDecoration(
          shape: categoryShape(cat, BorderSide(color: ring, width: 3.5)),
          color: verified ? fill : fill.withValues(alpha: 0.78),
          shadows: const [
            BoxShadow(color: Color(0x66000000), blurRadius: 4, offset: Offset(0, 1)),
          ],
        ),
        alignment: Alignment.center,
        child: Icon(categoryIcon(cat), size: 18, color: Colors.white),
      ),
    );
  }
}

/// Cluster bubble showing the marker count.
class ClusterBubble extends StatelessWidget {
  const ClusterBubble({super.key, required this.count});
  final int count;

  @override
  Widget build(BuildContext context) {
    final eco = context.eco;
    return Semantics(
      label: '$count',
      child: Container(
        decoration: BoxDecoration(
          shape: BoxShape.circle,
          color: eco.teal.withValues(alpha: 0.9),
          border: Border.all(color: Colors.white, width: 2),
          boxShadow: const [BoxShadow(color: Color(0x55000000), blurRadius: 5)],
        ),
        alignment: Alignment.center,
        child: Text(
          '$count',
          style: const TextStyle(
            color: Colors.white,
            fontWeight: FontWeight.w700,
            fontSize: 14,
          ),
        ),
      ),
    );
  }
}

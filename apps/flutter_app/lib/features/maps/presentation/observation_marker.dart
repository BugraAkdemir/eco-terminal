import 'package:flutter/material.dart';

import '../../../core/models/models.dart';
import '../../../core/theme/tokens.dart';

/// Map pin: category colour + icon fill, ring in the status colour.
class ObservationMarkerWidget extends StatelessWidget {
  const ObservationMarkerWidget({super.key, required this.observation});

  final Observation observation;

  @override
  Widget build(BuildContext context) {
    final eco = context.eco;
    final cat = observation.category;
    final fill = eco.categoryColor(cat);
    final ring = eco.statusColor(observation.status);
    final dashed = observation.status != ObservationStatus.verified &&
        observation.status != ObservationStatus.resolved;
    return Semantics(
      label: observation.displayTitle,
      button: true,
      child: Container(
        decoration: BoxDecoration(
          shape: BoxShape.circle,
          color: ring.withValues(alpha: 0.25),
          border: Border.all(color: ring, width: 3),
          boxShadow: const [
            BoxShadow(color: Color(0x55000000), blurRadius: 4, offset: Offset(0, 1)),
          ],
        ),
        padding: const EdgeInsets.all(3),
        child: DecoratedBox(
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            color: fill,
            border: dashed ? null : Border.all(color: Colors.white, width: 1),
          ),
          child: Icon(categoryIcon(cat), size: 16, color: Colors.white),
        ),
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
    return Container(
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        color: eco.teal.withValues(alpha: 0.85),
        border: Border.all(color: Colors.white.withValues(alpha: 0.9), width: 2),
        boxShadow: const [BoxShadow(color: Color(0x55000000), blurRadius: 5)],
      ),
      alignment: Alignment.center,
      child: Text(
        '$count',
        style: const TextStyle(
          color: Colors.white,
          fontWeight: FontWeight.w700,
          fontSize: 13,
        ),
      ),
    );
  }
}

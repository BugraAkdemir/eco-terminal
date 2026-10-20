import 'package:flutter/material.dart';

import '../core/l10n/strings.dart';
import '../core/models/enums.dart';
import '../core/theme/tokens.dart';

/// Status pill: distinct colour AND icon AND label for each of the 8
/// observation statuses.
class StatusBadge extends StatelessWidget {
  const StatusBadge({super.key, required this.status, this.compact = false});

  final ObservationStatus status;
  final bool compact;

  @override
  Widget build(BuildContext context) {
    final eco = context.eco;
    final color = eco.statusColor(status);
    final label = context.s.statusLabel(status);
    return Semantics(
      label: label,
      child: Container(
        padding: EdgeInsets.symmetric(
          horizontal: compact ? 6 : 10,
          vertical: compact ? 2 : 4,
        ),
        decoration: BoxDecoration(
          color: color.withValues(alpha: 0.14),
          borderRadius: BorderRadius.circular(999),
          border: Border.all(color: color.withValues(alpha: 0.7)),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(statusIcon(status), size: compact ? 12 : 14, color: color),
            const SizedBox(width: 4),
            Flexible(
              child: Text(
                label,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(
                  color: color,
                  fontSize: compact ? 11 : 12,
                  fontWeight: FontWeight.w600,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

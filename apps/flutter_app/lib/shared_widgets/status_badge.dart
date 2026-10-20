import 'package:flutter/material.dart';

import '../core/l10n/strings.dart';
import '../core/models/enums.dart';
import '../core/theme/tokens.dart';

/// Observation status as a "survey tag" stamp: icon + text + colour (never
/// colour alone). Tapping shows a one-line plain-language explanation
/// (disabled with [explain] false, e.g. inside tappable list rows).
class StatusBadge extends StatelessWidget {
  const StatusBadge({
    super.key,
    required this.status,
    this.compact = false,
    this.explain = true,
  });

  final ObservationStatus status;
  final bool compact;
  final bool explain;

  @override
  Widget build(BuildContext context) {
    final eco = context.eco;
    final s = context.s;
    final color = eco.statusColor(status);
    final label = s.statusLabel(status);
    final explanation = s.statusExplanation(status);

    final tag = Container(
      padding: EdgeInsets.symmetric(
        horizontal: compact ? 8 : 10,
        vertical: compact ? 3 : 5,
      ),
      decoration: ShapeDecoration(
        color: color.withValues(alpha: eco.isDark ? 0.14 : 0.10),
        // Squared "tag" with a thick border, like a stamped field label.
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(5),
          side: BorderSide(color: color, width: 1.6),
        ),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(statusIcon(status), size: compact ? 14 : 16, color: color),
          const SizedBox(width: 5),
          Flexible(
            child: Text(
              label,
              maxLines: 2,
              softWrap: true,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(
                color: color,
                fontSize: compact ? 13 : 14,
                height: 1.15,
                fontWeight: FontWeight.w700,
                letterSpacing: 0.2,
              ),
            ),
          ),
        ],
      ),
    );

    return Semantics(
      container: true,
      label: '${s.pick('Durum', 'Status')}: $label',
      hint: explanation,
      excludeSemantics: true,
      child: explain
          ? Tooltip(
              message: explanation,
              triggerMode: TooltipTriggerMode.tap,
              showDuration: const Duration(seconds: 5),
              child: tag,
            )
          : tag,
    );
  }
}

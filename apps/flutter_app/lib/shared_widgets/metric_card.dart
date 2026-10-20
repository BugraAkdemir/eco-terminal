import 'package:flutter/material.dart';

import '../core/theme/tokens.dart';
import 'panels.dart';

/// Big number + label (+ optional icon / accent colour).
class MetricCard extends StatelessWidget {
  const MetricCard({
    super.key,
    required this.label,
    required this.value,
    this.icon,
    this.accent,
    this.caption,
  });

  final String label;
  final String value;
  final IconData? icon;
  final Color? accent;
  final String? caption;

  @override
  Widget build(BuildContext context) {
    final eco = context.eco;
    final color = accent ?? eco.teal;
    return SolidPanel(
      padding: const EdgeInsets.all(Dimens.md),
      child: Row(
        children: [
          if (icon != null) ...[
            Container(
              width: 36,
              height: 36,
              decoration: BoxDecoration(
                color: color.withValues(alpha: 0.15),
                borderRadius: BorderRadius.circular(10),
              ),
              child: Icon(icon, size: 20, color: color),
            ),
            const SizedBox(width: Dimens.md),
          ],
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: [
                FittedBox(
                  fit: BoxFit.scaleDown,
                  alignment: Alignment.centerLeft,
                  child: Text(
                    value,
                    style: Theme.of(context).textTheme.titleLarge?.copyWith(
                          fontWeight: FontWeight.w700,
                          color: eco.text,
                        ),
                  ),
                ),
                Text(
                  label,
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                  style: Theme.of(context)
                      .textTheme
                      .bodySmall
                      ?.copyWith(color: eco.textMuted),
                ),
                if (caption != null)
                  Text(
                    caption!,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: Theme.of(context)
                        .textTheme
                        .labelSmall
                        ?.copyWith(color: eco.textMuted),
                  ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

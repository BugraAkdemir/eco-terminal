import 'package:flutter/material.dart';

import '../core/l10n/strings.dart';
import '../core/models/ai_models.dart';
import '../core/theme/tokens.dart';

/// Compact citation `[n] Title` for AI answers / feeds. Tapping calls
/// [onTap] (open observation / article); the tooltip shows type and snippet.
class SourceCitationChip extends StatelessWidget {
  const SourceCitationChip({super.key, required this.source, this.onTap});

  final SourceRef source;
  final VoidCallback? onTap;

  IconData get _icon {
    switch (source.type) {
      case 'observation':
        return Icons.visibility_outlined;
      case 'rss_article':
        return Icons.newspaper;
      case 'scientific':
        return Icons.science_outlined;
      case 'region_report':
        return Icons.map_outlined;
      case 'sql_fact':
        return Icons.storage_outlined;
      default:
        return Icons.link;
    }
  }

  @override
  Widget build(BuildContext context) {
    final eco = context.eco;
    final s = context.s;
    final tip = [
      s.sourceTypeLabel(source.type),
      if (source.date != null && source.date!.isNotEmpty) source.date!,
      if (source.snippet.isNotEmpty) source.snippet,
    ].join(' · ');
    return Tooltip(
      message: tip,
      child: ActionChip(
        onPressed: onTap,
        avatar: Icon(_icon, size: 14, color: eco.teal),
        label: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 220),
          child: Text(
            '${source.id} ${source.title}'.trim(),
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(fontSize: 12),
          ),
        ),
        visualDensity: VisualDensity.compact,
        side: BorderSide(color: eco.border),
        backgroundColor: eco.surfaceAlt.withValues(alpha: 0.5),
      ),
    );
  }
}

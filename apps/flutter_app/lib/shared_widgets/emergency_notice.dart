import 'package:flutter/material.dart';

import '../core/l10n/strings.dart';
import '../core/theme/tokens.dart';

/// Shown for injured wildlife / fire reports. States plainly that this app
/// does not forward anything to an official body; never claims otherwise.
class EmergencyNotice extends StatelessWidget {
  const EmergencyNotice({super.key});

  @override
  Widget build(BuildContext context) {
    final eco = context.eco;
    final s = context.s;
    return Semantics(
      container: true,
      liveRegion: true,
      label: s.emergencyNotice,
      child: Container(
        key: const ValueKey('emergency-notice'),
        padding: const EdgeInsets.all(Dimens.md),
        decoration: BoxDecoration(
          color: eco.critical.withValues(alpha: 0.12),
          borderRadius: BorderRadius.circular(Dimens.radiusSm),
          border: Border.all(color: eco.critical, width: 1.6),
        ),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Icon(Icons.phone_in_talk, color: eco.critical),
            const SizedBox(width: Dimens.md),
            Expanded(
              child: Text(
                s.emergencyNotice,
                style: const TextStyle(fontWeight: FontWeight.w700),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

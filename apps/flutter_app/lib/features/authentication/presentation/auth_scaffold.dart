import 'package:flutter/material.dart';

import '../../../core/l10n/strings.dart';
import '../../../core/theme/tokens.dart';
import '../../../shared_widgets/shared_widgets.dart';

/// Centered card used by login / register.
class AuthScaffold extends StatelessWidget {
  const AuthScaffold({
    super.key,
    required this.title,
    required this.child,
    this.notice,
  });

  final String title;
  final Widget child;
  final Widget? notice;

  @override
  Widget build(BuildContext context) {
    final eco = context.eco;
    final s = context.s;
    return Scaffold(
      body: Center(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(Dimens.lg),
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 420),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Row(
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: [
                    Container(
                      width: 40,
                      height: 40,
                      decoration: BoxDecoration(
                        gradient: LinearGradient(colors: [eco.green, eco.teal]),
                        borderRadius: BorderRadius.circular(11),
                      ),
                      child: const Icon(Icons.eco, color: Color(0xFF04210F)),
                    ),
                    const SizedBox(width: Dimens.md),
                    Text(
                      s.appName,
                      style: Theme.of(context)
                          .textTheme
                          .headlineSmall
                          ?.copyWith(fontWeight: FontWeight.w800),
                    ),
                  ],
                ),
                const SizedBox(height: Dimens.sm),
                Text(
                  s.appTagline,
                  textAlign: TextAlign.center,
                  style: TextStyle(color: eco.textMuted),
                ),
                const SizedBox(height: Dimens.xl),
                if (notice != null) ...[notice!, const SizedBox(height: Dimens.md)],
                SolidPanel(
                  padding: const EdgeInsets.all(Dimens.xl),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      Text(
                        title,
                        style: Theme.of(context)
                            .textTheme
                            .titleLarge
                            ?.copyWith(fontWeight: FontWeight.w700),
                      ),
                      const SizedBox(height: Dimens.lg),
                      child,
                    ],
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

/// Inline error banner for failed submissions.
class FormErrorBanner extends StatelessWidget {
  const FormErrorBanner({super.key, required this.message});
  final String message;

  @override
  Widget build(BuildContext context) {
    final eco = context.eco;
    return Container(
      key: const ValueKey('form-error'),
      padding: const EdgeInsets.all(Dimens.md),
      margin: const EdgeInsets.only(bottom: Dimens.md),
      decoration: BoxDecoration(
        color: eco.critical.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(Dimens.radiusSm),
        border: Border.all(color: eco.critical.withValues(alpha: 0.6)),
      ),
      child: Row(
        children: [
          Icon(Icons.error_outline, color: eco.critical, size: 18),
          const SizedBox(width: Dimens.sm),
          Expanded(child: Text(message)),
        ],
      ),
    );
  }
}

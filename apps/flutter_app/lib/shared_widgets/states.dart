import 'package:flutter/material.dart';

import '../core/errors/api_exception.dart';
import '../core/l10n/strings.dart';
import '../core/theme/tokens.dart';
import 'contour_texture.dart';

/// Honest empty state: icon, title, optional hint and action.
class EmptyState extends StatelessWidget {
  const EmptyState({
    super.key,
    required this.title,
    this.message,
    this.icon = Icons.inbox_outlined,
    this.action,
    this.compact = false,
  });

  final String title;
  final String? message;
  final IconData icon;
  final Widget? action;
  final bool compact;

  @override
  Widget build(BuildContext context) {
    final eco = context.eco;
    return ContourBackground(
      child: Center(
        child: Padding(
        padding: EdgeInsets.all(compact ? Dimens.lg : Dimens.xl),
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 360),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(icon, size: compact ? 32 : 44, color: eco.textMuted),
              const SizedBox(height: Dimens.md),
              Text(
                title,
                textAlign: TextAlign.center,
                style: Theme.of(context)
                    .textTheme
                    .titleMedium
                    ?.copyWith(fontWeight: FontWeight.w600),
              ),
              if (message != null) ...[
                const SizedBox(height: Dimens.sm),
                Text(
                  message!,
                  textAlign: TextAlign.center,
                  style: Theme.of(context)
                      .textTheme
                      .bodyMedium
                      ?.copyWith(color: eco.textMuted),
                ),
              ],
              if (action != null) ...[
                const SizedBox(height: Dimens.lg),
                action!,
              ],
            ],
          ),
        ),
      ),
      ),
    );
  }
}

/// Error with a friendly, localized message and a retry button.
class ErrorState extends StatelessWidget {
  const ErrorState({super.key, required this.error, this.onRetry, this.compact = false});

  final Object error;
  final VoidCallback? onRetry;
  final bool compact;

  @override
  Widget build(BuildContext context) {
    final s = context.s;
    final network = error is ApiException && (error as ApiException).isNetwork;
    return EmptyState(
      compact: compact,
      icon: network ? Icons.cloud_off_outlined : Icons.error_outline,
      title: s.errorText(error),
      action: onRetry == null
          ? null
          : OutlinedButton.icon(
              onPressed: onRetry,
              icon: const Icon(Icons.refresh, size: 18),
              label: Text(s.retry),
            ),
    );
  }
}

/// Animated shimmer rectangle. Honors `MediaQuery.disableAnimations`.
class ShimmerBox extends StatefulWidget {
  const ShimmerBox({
    super.key,
    this.width,
    this.height = 16,
    this.radius = 8,
  });

  final double? width;
  final double height;
  final double radius;

  @override
  State<ShimmerBox> createState() => _ShimmerBoxState();
}

class _ShimmerBoxState extends State<ShimmerBox>
    with SingleTickerProviderStateMixin {
  late final AnimationController _c = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 1400),
  );
  bool _started = false;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    final disabled = MediaQuery.maybeDisableAnimationsOf(context) ?? false;
    if (disabled) {
      _c.stop();
      _started = false;
    } else if (!_started) {
      _c.repeat();
      _started = true;
    }
  }

  @override
  void dispose() {
    _c.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final eco = context.eco;
    final base = eco.surfaceAlt;
    final hi = eco.isDark ? const Color(0xFF1B4561) : const Color(0xFFF7FBFC);
    return AnimatedBuilder(
      animation: _c,
      builder: (context, _) {
        final t = _c.value;
        return Container(
          width: widget.width,
          height: widget.height,
          decoration: BoxDecoration(
            borderRadius: BorderRadius.circular(widget.radius),
            gradient: LinearGradient(
              begin: Alignment(-1.0 + 2 * t - 0.6, 0),
              end: Alignment(-1.0 + 2 * t + 0.6, 0),
              colors: [base, hi, base],
            ),
          ),
        );
      },
    );
  }
}

/// Skeleton placeholder for list-like content.
class SkeletonLoader extends StatelessWidget {
  const SkeletonLoader({super.key, this.lines = 4, this.itemHeight = 56});

  final int lines;
  final double itemHeight;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      label: context.s.loading,
      child: Column(
        children: [
          for (var i = 0; i < lines; i++)
            Padding(
              padding: const EdgeInsets.only(bottom: Dimens.md),
              child: ShimmerBox(height: itemHeight, radius: Dimens.radius),
            ),
        ],
      ),
    );
  }
}

/// Banner shown when the last request failed with a transport error.
class OfflineBanner extends StatelessWidget {
  const OfflineBanner({super.key, this.onRetry, this.queueEnabled = false});
  final VoidCallback? onRetry;

  /// True when a feature module keeps unsent records on the device; only then
  /// the banner promises that records are stored on the phone.
  final bool queueEnabled;

  @override
  Widget build(BuildContext context) {
    final eco = context.eco;
    final s = context.s;
    return Material(
      color: eco.warning.withValues(alpha: 0.18),
      child: SafeArea(
        bottom: false,
        child: Padding(
          padding: const EdgeInsets.symmetric(
            horizontal: Dimens.lg,
            vertical: Dimens.sm,
          ),
          child: Row(
            children: [
              Icon(Icons.wifi_off, size: 18, color: eco.warning),
              const SizedBox(width: Dimens.sm),
              Expanded(
                child: Text(
                  queueEnabled ? s.offlineBannerQueue : s.offlineBanner,
                  style: TextStyle(color: eco.text, fontSize: 14),
                ),
              ),
              if (onRetry != null)
                TextButton(onPressed: onRetry, child: Text(s.retryConnection)),
            ],
          ),
        ),
      ),
    );
  }
}

void showSuccessSnack(BuildContext context, String message) {
  final eco = context.eco;
  ScaffoldMessenger.of(context)
    ..hideCurrentSnackBar()
    ..showSnackBar(
      SnackBar(
        content: Row(
          children: [
            Icon(Icons.check_circle, color: eco.green, size: 18),
            const SizedBox(width: Dimens.sm),
            Expanded(child: Text(message)),
          ],
        ),
      ),
    );
}

void showErrorSnack(BuildContext context, Object error) {
  final eco = context.eco;
  ScaffoldMessenger.of(context)
    ..hideCurrentSnackBar()
    ..showSnackBar(
      SnackBar(
        content: Row(
          children: [
            Icon(Icons.error_outline, color: eco.critical, size: 18),
            const SizedBox(width: Dimens.sm),
            Expanded(child: Text(context.s.errorText(error))),
          ],
        ),
      ),
    );
}

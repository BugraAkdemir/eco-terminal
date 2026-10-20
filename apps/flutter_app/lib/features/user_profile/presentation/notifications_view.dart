import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/l10n/format.dart';
import '../../../core/l10n/strings.dart';
import '../../../core/router/route_access.dart';
import '../../../core/theme/tokens.dart';
import '../../../shared_widgets/shared_widgets.dart';
import '../data/notifications_providers.dart';

/// List of own notifications with "mark as read". [shrinkWrap] for use inside
/// scrolling parents (profile page).
class NotificationsView extends ConsumerWidget {
  const NotificationsView({super.key, this.shrinkWrap = false, this.onNavigate});

  final bool shrinkWrap;

  /// Called after the user opens an observation from a notification.
  final VoidCallback? onNavigate;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = context.s;
    final eco = context.eco;
    final async = ref.watch(notificationsProvider);
    return async.when(
      loading: () => const Padding(
        padding: EdgeInsets.all(Dimens.lg),
        child: SkeletonLoader(lines: 3),
      ),
      error: (e, _) => ErrorState(
        error: e,
        compact: true,
        onRetry: () => ref.invalidate(notificationsProvider),
      ),
      data: (items) {
        if (items.isEmpty) {
          return EmptyState(
            compact: true,
            icon: Icons.notifications_none,
            title: s.noNotifications,
          );
        }
        return ListView.separated(
          shrinkWrap: shrinkWrap,
          physics: shrinkWrap ? const NeverScrollableScrollPhysics() : null,
          itemCount: items.length,
          separatorBuilder: (_, _) => const Divider(),
          itemBuilder: (context, i) {
            final n = items[i];
            return ListTile(
              dense: true,
              leading: Icon(
                n.isRead ? Icons.notifications_none : Icons.notifications_active,
                color: n.isRead ? eco.textMuted : eco.teal,
              ),
              title: Text(
                n.title,
                style: TextStyle(
                  fontWeight: n.isRead ? FontWeight.w400 : FontWeight.w700,
                ),
              ),
              subtitle: Text(
                [
                  if (n.body.isNotEmpty) n.body,
                  formatDateTime(context, n.createdAt),
                ].where((e) => e.isNotEmpty).join('\n'),
              ),
              isThreeLine: n.body.isNotEmpty,
              trailing: n.isRead
                  ? null
                  : IconButton(
                      tooltip: s.markRead,
                      icon: const Icon(Icons.done, size: 20),
                      onPressed: () async {
                        try {
                          await markNotificationRead(ref, n.id);
                        } catch (e) {
                          if (context.mounted) showErrorSnack(context, e);
                        }
                      },
                    ),
              onTap: n.observationId == null
                  ? null
                  : () {
                      onNavigate?.call();
                      context.push(AppPaths.observation(n.observationId!));
                    },
            );
          },
        );
      },
    );
  }
}

/// Dialog opened from the header bell.
Future<void> showNotificationsDialog(BuildContext context) {
  final s = context.s;
  return showDialog<void>(
    context: context,
    builder: (ctx) => Dialog(
      alignment: Alignment.topRight,
      insetPadding: const EdgeInsets.fromLTRB(16, 72, 16, 16),
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 420, maxHeight: 520),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(Dimens.lg, Dimens.sm, Dimens.sm, 0),
              child: Row(
                children: [
                  Expanded(
                    child: Text(
                      s.notifications,
                      style: Theme.of(ctx).textTheme.titleMedium,
                    ),
                  ),
                  IconButton(
                    tooltip: s.close,
                    onPressed: () => Navigator.of(ctx).pop(),
                    icon: const Icon(Icons.close),
                  ),
                ],
              ),
            ),
            const Divider(),
            Flexible(
              child: NotificationsView(
                onNavigate: () => Navigator.of(ctx).pop(),
              ),
            ),
          ],
        ),
      ),
    ),
  );
}

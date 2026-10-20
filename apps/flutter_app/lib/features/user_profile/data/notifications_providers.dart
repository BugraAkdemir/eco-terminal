import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/providers.dart';
import '../../../core/models/models.dart';
import '../../../core/session/session_controller.dart';

/// Own notifications (empty list for anonymous users, no request made).
final notificationsProvider =
    FutureProvider<List<AppNotification>>((ref) async {
  final authed = ref.watch(sessionProvider.select((s) => s.isAuthenticated));
  if (!authed) return const [];
  return ref.watch(notificationsApiProvider).list();
});

final unreadCountProvider = Provider<int>((ref) {
  final list = ref.watch(notificationsProvider).value ?? const [];
  return list.where((n) => !n.isRead).length;
});

/// Marks as read on the server and refreshes the list.
Future<void> markNotificationRead(WidgetRef ref, String id) async {
  await ref.read(notificationsApiProvider).markRead(id);
  ref.invalidate(notificationsProvider);
}

/// Refreshes notifications every [interval] while signed in. Watch this from
/// the app shell. The timer is cancelled when the provider is disposed.
final notificationsPollerProvider = Provider<void>((ref) {
  final authed = ref.watch(sessionProvider.select((s) => s.isAuthenticated));
  if (!authed) return;
  final timer = Timer.periodic(
    const Duration(seconds: 60),
    (_) => ref.invalidate(notificationsProvider),
  );
  ref.onDispose(timer.cancel);
});

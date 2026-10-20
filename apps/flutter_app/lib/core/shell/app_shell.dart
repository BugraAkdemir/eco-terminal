import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../features/maps/data/map_providers.dart';
import '../../features/user_profile/data/notifications_providers.dart';
import '../api/providers.dart';
import '../l10n/strings.dart';
import '../router/route_access.dart';
import '../session/session_controller.dart';
import '../theme/tokens.dart';
import '../../shared_widgets/shared_widgets.dart';
import 'app_header.dart';

/// Frame around every in-app page: offline banner, header, content and (on
/// mobile) the bottom navigation Harita / Gözlem Ekle / AI / Profil.
class AppShell extends ConsumerWidget {
  const AppShell({super.key, required this.child});

  final Widget child;

  static int _indexFor(String path) {
    if (path.startsWith(AppPaths.newObservation)) return 1;
    if (path.startsWith(AppPaths.ai)) return 2;
    if (path.startsWith(AppPaths.profile)) return 3;
    return 0;
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    ref.watch(notificationsPollerProvider);
    final online = ref.watch(networkOnlineProvider);
    final width = MediaQuery.sizeOf(context).width;
    final mobile = screenSizeForWidth(width) == ScreenSize.mobile;
    final s = context.s;
    final path = GoRouterState.of(context).uri.path;

    return Scaffold(
      body: Column(
        children: [
          if (!online) OfflineBanner(onRetry: () => _checkConnection(ref)),
          const AppHeader(),
          Expanded(child: child),
        ],
      ),
      bottomNavigationBar: mobile
          ? NavigationBar(
              selectedIndex: _indexFor(path),
              onDestinationSelected: (i) {
                switch (i) {
                  case 0:
                    context.go(AppPaths.home);
                  case 1:
                    context.go(AppPaths.newObservation);
                  case 2:
                    context.go(AppPaths.ai);
                  case 3:
                    context.go(AppPaths.profile);
                }
              },
              destinations: [
                NavigationDestination(
                  icon: const Icon(Icons.map_outlined),
                  selectedIcon: const Icon(Icons.map),
                  label: s.navMap,
                ),
                NavigationDestination(
                  icon: const Icon(Icons.add_circle_outline),
                  selectedIcon: const Icon(Icons.add_circle),
                  label: s.navAddObservation,
                ),
                NavigationDestination(
                  icon: const Icon(Icons.auto_awesome_outlined),
                  selectedIcon: const Icon(Icons.auto_awesome),
                  label: s.navAi,
                ),
                NavigationDestination(
                  icon: const Icon(Icons.person_outline),
                  selectedIcon: const Icon(Icons.person),
                  label: s.navProfile,
                ),
              ],
            )
          : null,
    );
  }

  Future<void> _checkConnection(WidgetRef ref) async {
    try {
      await ref.read(systemApiProvider).health();
    } catch (_) {
      return;
    }
    // Back online: refresh what depends on the server.
    ref.invalidate(viewportObservationsProvider);
    ref.invalidate(regionShapesProvider);
    ref.invalidate(notificationsProvider);
    await ref.read(sessionProvider.notifier).refreshUser();
  }
}

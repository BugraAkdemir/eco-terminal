import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../features/authentication/presentation/login_page.dart';
import '../../features/authentication/presentation/register_page.dart';
import '../../features/maps/presentation/map_home_page.dart';
import '../../features/maps/presentation/observation_page.dart';
import '../../features/user_profile/presentation/profile_page.dart';
import '../../shared_widgets/shared_widgets.dart';
import '../extensions/extension_registry.dart';
import '../l10n/strings.dart';
import '../session/session_controller.dart';
import '../shell/app_shell.dart';
import 'route_access.dart';

final rootNavigatorKey = GlobalKey<NavigatorState>(debugLabel: 'root');
final shellNavigatorKey = GlobalKey<NavigatorState>(debugLabel: 'shell');

/// Placeholder for an extension slot nobody registered yet (honest, no fake UI).
class _SlotPlaceholder extends StatelessWidget {
  const _SlotPlaceholder();

  @override
  Widget build(BuildContext context) {
    final s = context.s;
    return EmptyState(
      icon: Icons.extension_off_outlined,
      title: s.moduleNotConfigured,
      message: s.moduleNotConfiguredHint,
    );
  }
}

class _SplashPage extends StatelessWidget {
  const _SplashPage();

  @override
  Widget build(BuildContext context) =>
      const Scaffold(body: Center(child: CircularProgressIndicator()));
}

class _ForbiddenPage extends StatelessWidget {
  const _ForbiddenPage();

  @override
  Widget build(BuildContext context) {
    final s = context.s;
    return EmptyState(
      icon: Icons.lock_outline,
      title: s.forbiddenTitle,
      action: FilledButton(
        onPressed: () => context.go(AppPaths.home),
        child: Text(s.goHome),
      ),
    );
  }
}

/// Notifies go_router when the session changes so redirects re-run.
class _SessionListenable extends ChangeNotifier {
  void ping() => notifyListeners();
}

final routerProvider = Provider<GoRouter>((ref) {
  final registry = ref.watch(extensionRegistryProvider);
  final listenable = _SessionListenable();
  ref.listen<SessionState>(sessionProvider, (_, _) => listenable.ping());
  ref.onDispose(listenable.dispose);

  Widget slot(String path, ExtensionRouteBuilder fallback, BuildContext c, GoRouterState st) {
    final ext = registry.route(path);
    return (ext?.builder ?? fallback)(c, st);
  }

  final router = GoRouter(
    navigatorKey: rootNavigatorKey,
    initialLocation: AppPaths.home,
    refreshListenable: listenable,
    redirect: (context, state) => computeRedirect(
      session: ref.read(sessionProvider),
      location: state.uri,
      accessFor: registry.accessFor,
    ),
    routes: [
      GoRoute(path: AppPaths.splash, builder: (_, _) => const _SplashPage()),
      GoRoute(
        path: AppPaths.login,
        builder: (_, state) => LoginPage(
          from: state.uri.queryParameters['from'],
          expired: state.uri.queryParameters['expired'] == '1',
        ),
      ),
      GoRoute(
        path: AppPaths.register,
        builder: (_, state) =>
            RegisterPage(from: state.uri.queryParameters['from']),
      ),
      ShellRoute(
        navigatorKey: shellNavigatorKey,
        builder: (context, state, child) => AppShell(child: child),
        routes: [
          GoRoute(
            path: AppPaths.home,
            builder: (_, _) => const MapHomePage(),
          ),
          GoRoute(path: AppPaths.ai, builder: (_, _) => const AiPage()),
          GoRoute(
            path: AppPaths.profile,
            builder: (_, _) => const ProfilePage(),
          ),
          GoRoute(
            path: AppPaths.forbidden,
            builder: (_, _) => const _ForbiddenPage(),
          ),
          // Extension slots. Static paths are declared before ':id'.
          GoRoute(
            path: AppPaths.newObservation,
            builder: (c, st) =>
                slot(AppPaths.newObservation, (_, _) => const _SlotPlaceholder(), c, st),
          ),
          GoRoute(
            path: AppPaths.expert,
            builder: (c, st) =>
                slot(AppPaths.expert, (_, _) => const _SlotPlaceholder(), c, st),
          ),
          GoRoute(
            path: AppPaths.observationDetailPattern,
            builder: (c, st) => slot(
              AppPaths.observationDetailPattern,
              (_, state) => ObservationPage(id: state.pathParameters['id']!),
              c,
              st,
            ),
          ),
          // Any other routes features registered.
          for (final r in registry.routes)
            if (![
              AppPaths.newObservation,
              AppPaths.expert,
              AppPaths.observationDetailPattern,
            ].contains(r.path))
              GoRoute(path: r.path, builder: r.builder),
        ],
      ),
    ],
  );
  ref.onDispose(router.dispose);
  return router;
});

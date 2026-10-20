import '../models/enums.dart';
import '../session/session_controller.dart';

/// Canonical route paths.
class AppPaths {
  const AppPaths._();

  static const splash = '/splash';
  static const home = '/';
  static const login = '/login';
  static const register = '/register';
  static const profile = '/profile';
  static const ai = '/ai';
  static const forbidden = '/forbidden';

  /// Extension slots (default placeholder until a feature registers).
  static const newObservation = '/observations/new';
  static const expert = '/expert';
  static const observationDetailPattern = '/observations/:id';

  static String observation(String id) => '/observations/$id';
}

/// Who may open a route.
class RouteAccess {
  const RouteAccess({this.requiresAuth = false, this.minRole});

  static const public = RouteAccess();
  static const authenticated = RouteAccess(requiresAuth: true);

  final bool requiresAuth;

  /// Minimum role; roles are ordered citizen < researcher/reviewer < admin,
  /// where reviewer and researcher are siblings with distinct powers.
  final UserRole? minRole;

  bool allows(UserRole role) {
    final min = minRole;
    if (min == null) return true;
    if (role == UserRole.admin) return true;
    return role == min;
  }
}

/// Access rules for paths owned by the foundation. Extension routes pass
/// their own [RouteAccess] when registering.
RouteAccess defaultAccessFor(String path) {
  if (path == AppPaths.profile) return RouteAccess.authenticated;
  if (path == AppPaths.newObservation) return RouteAccess.authenticated;
  if (path == AppPaths.expert || path.startsWith('${AppPaths.expert}/')) {
    // Reviewer or admin only (researchers cannot decide on records).
    return const RouteAccess(requiresAuth: true, minRole: UserRole.reviewer);
  }
  return RouteAccess.public;
}

/// Pure redirect decision used by go_router; unit-testable.
/// Returns the location to redirect to, or null to stay.
String? computeRedirect({
  required SessionState session,
  required Uri location,
  required RouteAccess Function(String path) accessFor,
}) {
  final path = location.path;

  if (session.isRestoring) {
    return path == AppPaths.splash ? null : AppPaths.splash;
  }
  if (path == AppPaths.splash) return AppPaths.home;

  final authPage = path == AppPaths.login || path == AppPaths.register;

  // A 401 cleared the session: send the user to login once.
  if (session.expired && !authPage) {
    return Uri(
      path: AppPaths.login,
      queryParameters: {'expired': '1'},
    ).toString();
  }

  if (session.isAuthenticated && authPage) {
    return location.queryParameters['from'] ?? AppPaths.home;
  }

  final access = accessFor(path);
  if (access.requiresAuth && !session.isAuthenticated) {
    return Uri(
      path: AppPaths.login,
      queryParameters: {'from': location.toString()},
    ).toString();
  }
  final role = session.user?.role;
  if (access.minRole != null && role != null && !access.allows(role)) {
    return AppPaths.forbidden;
  }
  return null;
}

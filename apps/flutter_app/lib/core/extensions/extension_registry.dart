import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../app_extensions.dart';
import '../router/route_access.dart';

/// Ids of the side-panel tabs. The first two are built in; the rest are
/// extension slots with an honest "not configured" default.
class PanelTabIds {
  const PanelTabIds._();

  static const information = 'information';
  static const observations = 'observations';
  static const aiAssistant = 'ai_assistant';
  static const officialUpdates = 'official_updates';
  static const simulation = 'simulation';

  static const all = [
    information,
    observations,
    aiAssistant,
    officialUpdates,
    simulation,
  ];
}

/// Map layer ids that extension overlays may attach to.
class MapOverlayLayerIds {
  const MapOverlayLayerIds._();

  /// The "Official Announcements" toggle in the layer menu.
  static const officialAnnouncements = 'official_announcements';
}

/// Builds a flutter_map layer (e.g. a `MarkerLayer`) that is inserted into the
/// map above the base layers while the toggle for [layerId] is on. The widget
/// is placed directly inside `FlutterMap.children`, so it may be a
/// `Consumer` returning a layer.
class MapOverlaySpec {
  const MapOverlaySpec({required this.layerId, required this.builder});

  final String layerId;
  final WidgetBuilder builder;
}

typedef ExtensionRouteBuilder = Widget Function(BuildContext context, GoRouterState state);

class ExtensionRoute {
  const ExtensionRoute({
    required this.path,
    required this.builder,
    this.access = RouteAccess.authenticated,
  });
  final String path;
  final ExtensionRouteBuilder builder;
  final RouteAccess access;
}

/// Collects what later feature modules plug into the foundation. Features add
/// files in their own directories and ONE registration line in
/// `lib/app_extensions.dart`:
///
/// ```dart
/// void registerExtensions(ExtensionRegistry r) {
///   r.registerPanelTab(PanelTabIds.aiAssistant, (c) => const AiAssistantTab());
///   r.registerRoute(AppPaths.newObservation, (c, s) => const ObservationWizardPage());
/// }
/// ```
class ExtensionRegistry {
  final Map<String, WidgetBuilder> _panelTabs = {};
  final Map<String, ExtensionRoute> _routes = {};
  final List<MapOverlaySpec> _overlays = [];

  /// Replaces the default content of a panel tab (see [PanelTabIds]).
  void registerPanelTab(String tabId, WidgetBuilder builder) =>
      _panelTabs[tabId] = builder;

  /// Registers (or replaces) a full-screen route inside the app shell.
  /// Use [AppPaths.newObservation], [AppPaths.expert] or
  /// [AppPaths.observationDetailPattern] to replace the built-in defaults.
  void registerRoute(
    String path,
    ExtensionRouteBuilder builder, {
    RouteAccess? access,
  }) {
    _routes[path] = ExtensionRoute(
      path: path,
      builder: builder,
      access: access ?? defaultAccessFor(path),
    );
  }

  /// Adds a map overlay layer tied to a layer toggle id.
  void registerMapOverlay(MapOverlaySpec spec) => _overlays.add(spec);

  WidgetBuilder? panelTab(String tabId) => _panelTabs[tabId];
  bool hasPanelTab(String tabId) => _panelTabs.containsKey(tabId);

  ExtensionRoute? route(String path) => _routes[path];
  Iterable<ExtensionRoute> get routes => _routes.values;

  List<MapOverlaySpec> overlaysFor(String layerId) =>
      _overlays.where((o) => o.layerId == layerId).toList();
  bool hasOverlayFor(String layerId) =>
      _overlays.any((o) => o.layerId == layerId);

  RouteAccess accessFor(String path) {
    for (final r in _routes.values) {
      if (_matches(r.path, path)) return r.access;
    }
    return defaultAccessFor(path);
  }

  static bool _matches(String pattern, String path) {
    if (pattern == path) return true;
    final p = pattern.split('/');
    final q = path.split('/');
    if (p.length != q.length) return false;
    for (var i = 0; i < p.length; i++) {
      if (p[i].startsWith(':')) continue;
      if (p[i] != q[i]) return false;
    }
    return true;
  }
}

/// The registry used by the app. Tests override this provider with their own.
final extensionRegistryProvider = Provider<ExtensionRegistry>((ref) {
  final registry = ExtensionRegistry();
  registerExtensions(registry);
  return registry;
});

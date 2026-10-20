import 'core/extensions/extension_registry.dart';

/// Single registration point for feature modules that are built on top of the
/// foundation. Each feature adds its own files under `lib/features/<name>/`
/// and ONE line here. Nothing registered = honest "module not configured"
/// placeholders (no fake data).
///
/// Slots:
///  - Panel tabs: PanelTabIds.aiAssistant / officialUpdates / simulation
///      r.registerPanelTab(PanelTabIds.aiAssistant, (context) => const AiTab());
///  - Routes: AppPaths.newObservation ('/observations/new'),
///      AppPaths.expert ('/expert'), AppPaths.observationDetailPattern
///      r.registerRoute(AppPaths.expert, (context, state) => const ExpertPage());
///  - Map overlays (e.g. official announcements markers):
///      r.registerMapOverlay(MapOverlaySpec(
///        layerId: MapOverlayLayerIds.officialAnnouncements,
///        builder: (context) => const OfficialAnnouncementsLayer()));
void registerExtensions(ExtensionRegistry r) {
  // AGENT-SLOT (one line per feature module, keep alphabetical):
}

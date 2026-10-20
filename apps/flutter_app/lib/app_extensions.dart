import 'core/extensions/extension_registry.dart';
import 'core/router/route_access.dart';
import 'features/observations/presentation/observation_wizard.dart';
import 'features/observations/presentation/queue_page.dart';
import 'features/expert_dashboard/presentation/expert_page.dart';
import 'features/intelligence/presentation/intelligence_tabs.dart';

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
  r.registerRoute(
    AppPaths.newObservation,
    (c, s) => ObservationWizardPage(draftId: s.uri.queryParameters['draft']),
  );
  r.registerRoute('/field-queue', (c, s) => const FieldQueuePage());
  r.registerRoute(AppPaths.expert, (c, s) => const ExpertPage());
  r.registerPanelTab(PanelTabIds.aiAssistant, (c) => const AssistantTab());
  r.registerPanelTab(PanelTabIds.officialUpdates, (c) => const FeedsTab());
  r.registerPanelTab(PanelTabIds.simulation, (c) => const SimulationsTab());
}

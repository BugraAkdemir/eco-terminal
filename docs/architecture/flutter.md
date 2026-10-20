# Flutter client (`apps/flutter_app`)

Foundation for every client feature. Source of truth for the API is `docs/api/openapi.yaml`; this file documents how the app is wired and how later modules plug in.

## Run / build / test

```bash
cd apps/flutter_app
flutter run -d linux --dart-define=API_BASE_URL=http://localhost:8080/api/v1
flutter analyze && flutter test
flutter build linux
```

Build-time defines: `API_BASE_URL` (default `http://localhost:8080/api/v1`), `TILE_URL` (default OSM raster tiles).

> **OSM tiles.** `tile.openstreetmap.org` is for light use only (OSM tile usage policy). Production needs a tile provider or an own tile server: set `--dart-define=TILE_URL=https://.../{z}/{x}/{y}.png`. Attribution ("© OpenStreetMap contributors") is always visible on the map.

## Layout

```
lib/
  main.dart, app.dart          entry, MaterialApp.router (theme, locale, router)
  app_extensions.dart          THE registration point for later modules
  core/
    config/                    AppConfig (API base URL), MapConfig
    errors/                    ApiException(code, message, status, details)
    api/                       ApiClient (Dio + envelope), providers.dart, services/*_api.dart
    models/                    immutable models with fromJson/toJson for every schema
    session/                   SessionController (restore, login, logout, 401, delete account)
    storage/                   TokenStore (secure storage + prefs fallback), locale/theme prefs
    l10n/                      AppStrings (tr default + en), formatting helpers
    theme/                     EcoColors tokens (dark, light, high-contrast), AppTheme, fonts
    router/                    route_access.dart (guards), app_router.dart
    shell/                     AppShell, AppHeader, SidePanel, panel tabs, search, offline queue hook
    extensions/                ExtensionRegistry
  shared_widgets/              GlassPanel, SolidPanel, MetricCard, FilterChipRow, StatusBadge,
                               EmptyState, ErrorState, SkeletonLoader, OfflineBanner, SourceCitationChip,
                               SectionHeader, EmergencyNotice, StickyBottomBar, Reveal, ContourBackground
  features/
    authentication/            login/register, validators
    maps/                      map, regions, observations tabs, detail card, providers (data/, domain/)
    user_profile/              profile, notifications
```

State management is Riverpod 3 (`Notifier`/`AsyncNotifier`/`FutureProvider`); routing is go_router.

## API layer

`ApiClient` wraps Dio and understands the envelope: success `{data, meta}` -> `ApiEnvelope`, failure `{error:{code,message,details}}` -> `ApiException`. Transport failures become `network_unavailable` / `timeout` (friendly localized text via `AppStrings.errorText`). An authenticated request answered with 401 calls `SessionController.handleUnauthorized()` (session cleared, router redirects to `/login?expired=1`); a 401 on an anonymous call (wrong password) does not. Every transport failure/success flips `networkOnlineProvider`, which drives the offline banner.

Typed services (one per tag, all endpoints of the contract): `AuthApi`, `SystemApi`, `ObservationsApi`, `MediaApi` (multipart upload), `RegionsApi`, `ReviewApi`, `NotificationsApi`, `AiApi`, `FeedsApi` (+admin), `SimulationApi`, `AdminApi`. Providers: `authApiProvider`, `observationsApiProvider`, ... in `core/api/providers.dart`. Feature code never touches Dio directly.

## Responsive shell

Breakpoints: `<700` mobile, `700-1100` tablet, `>1100` desktop (`screenSizeForWidth`).

- Desktop: header + resizable (drag handle) / collapsible left "Information & Intelligence" panel (28% default) + map.
- Tablet: map with an overlay panel that slides in/out.
- Mobile: map, panel as bottom sheet ("Bölge bilgisi" / "Liste" buttons), bottom nav Harita / Gözlem Ekle / AI / Profil.

The panel has 5 tabs: Information, Observations (both real), AI Assistant, Official Updates, Simulation (extension slots).

### Shared context

`selectedRegionProvider` (features/maps/domain/map_state.dart) is the shared UX context. Tapping a region polygon, the region search, or a quick-select chip sets it; every tab and the future AI tab read it (`ref.watch(selectedRegionProvider)`). The visible "Aktif bölge" chip shows it. Also shared: `dateRangeProvider` / `resolvedRangeProvider`, `categoryFilterProvider`, `mapLayersProvider`, `selectedObservationIdProvider`, `mapFocusProvider` (one-shot camera requests: `ref.read(mapFocusProvider.notifier).flyTo(...)` / `.fitBounds(bbox)`).

## Extension points (`lib/app_extensions.dart`)

A later module adds files under its own `lib/features/<name>/` and one line in `registerExtensions`:

```dart
void registerExtensions(ExtensionRegistry r) {
  // panel tab content (ids in PanelTabIds)
  r.registerPanelTab(PanelTabIds.aiAssistant, (context) => const AiAssistantTab());
  // routes (replace the defaults for these three, or add any new path)
  r.registerRoute(AppPaths.newObservation, (context, state) => const ObservationWizardPage());
  r.registerRoute(AppPaths.expert, (context, state) => const ExpertDashboardPage());
  r.registerRoute(AppPaths.observationDetailPattern, (context, state) => ObservationDetailPage(id: state.pathParameters['id']!));
  // a layer inserted into flutter_map while the "Resmi Duyurular" toggle is on
  r.registerMapOverlay(MapOverlaySpec(
      layerId: MapOverlayLayerIds.officialAnnouncements,
      builder: (context) => const OfficialAnnouncementsLayer()));
}
```

- Unregistered tabs/routes render a clear `Bu modül henüz yapılandırılmadı` empty state (no fake data). `/observations/:id` has a real default (`ObservationPage`).
- Default access: `/observations/new` needs login; `/expert` needs reviewer or admin (`defaultAccessFor`); pass `access: RouteAccess(...)` to `registerRoute` for others.
- Tab/route/overlay builders are plain widgets: they can be `ConsumerWidget`s and use all the shared providers and `core/api` services.
- Extra strings: use `context.s.pick('Türkçe', 'English')` so `core/l10n/strings.dart` stays conflict-free.
- Offline queue: the wizard calls `ref.read(offlineQueueProvider.notifier).setSupported(true)` / `.setPending(n)`; only then does the offline banner say records are kept on the phone, and the Profil tab shows the pending badge.
- Tests override `extensionRegistryProvider`.

## Map

`flutter_map` + OSM tiles (dark mode via the `darkModeTileBuilder` colour filter, toggle in the map controls and layer menu). Camera moves are debounced (450 ms) into `viewportProvider`; the provider refetches `GET /observations?bbox=...&page_size=200` (server-capped; a note appears when more records exist). The marker list is memoized and only rebuilt when the data/filters change, clustering is `flutter_map_marker_cluster`. Markers differ by shape + icon per category and a status-colour ring. Density view replaces markers with a translucent circle layer. Region polygons come from `GET /regions?geometry=true` (parsed once); map taps use local point-in-polygon and pick the finest region. The detail card shows media, the user's guess, verified species, status, an AI suggestion labelled unverified, a history timeline and a "Konum genelleştirildi" badge.

## Plugins (Linux)

All of these compile in `flutter build linux`: flutter_map, flutter_map_marker_cluster, dio, riverpod, go_router, fl_chart, shared_preferences, flutter_secure_storage (libsecret), geolocator, image_picker, url_launcher. Android/iOS/web builds were NOT verified in this environment (no Android SDK / Chrome).

- Token storage: `flutter_secure_storage` (libsecret on Linux). If the keystore is unavailable (headless Linux without a Secret Service) it silently falls back to `shared_preferences` - weaker at-rest protection, acceptable because the token is an opaque, server-revocable session token.
- `geolocator` is only used for the my-location button; a denied/unavailable location shows a calm message, the map stays usable.

## Design ("Saha Atlası")

Tokens live in `core/theme/tokens.dart` (`context.eco`): dark navy (default), light and an outdoor high-contrast theme (user menu). All three are covered by a WCAG test (>= 4.5:1 accents, >= 7:1 body). Status is a "survey tag" (icon + text + colour, never colour alone; tapping shows a one-line explanation), markers use shape + icon, thin borders, faint static contour texture. Body text is 16sp, touch targets 48dp, motion 150-250 ms and disabled with OS reduce-motion; the shell survives 200% text scale (tested).

Fonts: Fraunces (headings), Atkinson Hyperlegible Next (body/UI; keeps the `AtkinsonHyperlegible` Flutter alias), IBM Plex Mono (coordinates, ids, counts) are the configured families (`kDisplayFont`, `kBodyFont`, `kMonoFont`) bundled under `assets/fonts/` and declared in `pubspec.yaml`; no runtime font download is needed. Each family includes its OFL license. See `assets/fonts/README.md` for provenance and Turkish glyph coverage verification. System fallbacks remain for unsupported scripts.

## Known gaps

Observation wizard, expert dashboard, AI assistant, official feeds and simulation screens are other modules (extension slots above). Custom region drawing (stretch goal) is not implemented. The list view next to the map is the Observations tab (mobile: "Liste" button).

## Implemented feature modules (Bio Radar)

`app_extensions.dart` registers the six-step field wizard, `/field-queue`, reviewer desk and assistant/feed/scenario tabs. The profile links to the local queue. ObservationCreate sends `client_ref`; partial media upload progress survives restart. Queue storage is scoped by account, native JSON is written via flush + atomic rename in app support storage, web uses checked browser preferences. Retry uses bounded exponential backoff while the app is open; logout prevents another account sending the previous account's records. Photos are retained until all uploads succeed; save failures are visible.

The assistant passes selected region/dates and renders sources, SQL summaries and unverified AI labels. Feeds show source health and original links. Scenarios call the real API, retain missing climate points as chart gaps, show units, assumptions and attribution, and compare stored scenarios. Analytics charts remain in the existing Information tab. Native window and platform display names are Bio Radar; package IDs, binary name and ECO_* server variables remain stable.

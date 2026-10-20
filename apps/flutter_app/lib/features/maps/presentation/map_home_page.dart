import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/extensions/extension_registry.dart';
import '../../../core/l10n/strings.dart';
import '../../../core/router/route_access.dart';
import '../../../core/shell/panel_tabs.dart';
import '../../../core/shell/side_panel.dart';
import '../../../core/theme/tokens.dart';
import '../domain/map_state.dart';
import 'map_controls.dart';
import 'map_stack.dart';

/// The home screen: map plus the Information & Intelligence panel.
/// - desktop (>1100): resizable, collapsible left panel + map
/// - tablet (700-1100): panel as a collapsible overlay on the map
/// - mobile (<700): map; panel opens as a bottom sheet
class MapHomePage extends ConsumerWidget {
  const MapHomePage({super.key, this.initialTab});

  /// Optional panel tab to select (used by `/ai` on wide screens).
  final String? initialTab;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return LayoutBuilder(
      builder: (context, c) {
        switch (screenSizeForWidth(c.maxWidth)) {
          case ScreenSize.desktop:
            return _DesktopLayout(width: c.maxWidth);
          case ScreenSize.tablet:
            return const _TabletLayout();
          case ScreenSize.mobile:
            return const _MobileLayout();
        }
      },
    );
  }
}

/// "Gözlem Ekle" button over the map on wide layouts.
class _AddObservationFab extends StatelessWidget {
  const _AddObservationFab();

  @override
  Widget build(BuildContext context) {
    return Positioned(
      bottom: 36,
      left: 0,
      right: 0,
      child: Center(
        child: FloatingActionButton.extended(
          heroTag: 'add-observation',
          onPressed: () => context.go(AppPaths.newObservation),
          icon: const Icon(Icons.add_a_photo_outlined),
          label: Text(context.s.navAddObservation),
        ),
      ),
    );
  }
}

class _DesktopLayout extends ConsumerWidget {
  const _DesktopLayout({required this.width});
  final double width;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final open = ref.watch(desktopPanelOpenProvider);
    final fraction = ref.watch(panelFractionProvider);
    final eco = context.eco;
    final s = context.s;

    return Row(
      children: [
        if (open) ...[
          SizedBox(
            key: const ValueKey('desktop-panel'),
            width: width * fraction,
            child: SidePanel(
              onCollapse: () =>
                  ref.read(desktopPanelOpenProvider.notifier).set(false),
            ),
          ),
          MouseRegion(
            cursor: SystemMouseCursors.resizeColumn,
            child: GestureDetector(
              key: const ValueKey('panel-resize-handle'),
              behavior: HitTestBehavior.opaque,
              onHorizontalDragUpdate: (d) => ref
                  .read(panelFractionProvider.notifier)
                  .set(fraction + d.delta.dx / width),
              child: Container(
                width: 8,
                color: eco.sidebar,
                alignment: Alignment.center,
                child: Container(width: 2, height: 48, color: eco.border),
              ),
            ),
          ),
        ] else
          Container(
            key: const ValueKey('desktop-panel-rail'),
            width: 44,
            color: eco.sidebar,
            alignment: Alignment.topCenter,
            padding: const EdgeInsets.only(top: Dimens.sm),
            child: IconButton(
              tooltip: s.expandPanel,
              onPressed: () =>
                  ref.read(desktopPanelOpenProvider.notifier).set(true),
              icon: const Icon(Icons.chevron_right),
            ),
          ),
        const Expanded(
          child: Stack(
            children: [
              Positioned.fill(child: MapStack()),
              _AddObservationFab(),
            ],
          ),
        ),
      ],
    );
  }
}

class _TabletLayout extends ConsumerWidget {
  const _TabletLayout();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final open = ref.watch(tabletPanelOpenProvider);
    final eco = context.eco;
    final s = context.s;
    const panelWidth = 360.0;

    return Stack(
      children: [
        const Positioned.fill(child: MapStack()),
        const _AddObservationFab(),
        if (open)
          Positioned(
            key: const ValueKey('tablet-panel'),
            left: 0,
            top: 0,
            bottom: 0,
            width: panelWidth,
            child: Material(
              elevation: 8,
              child: SidePanel(
                onCollapse: () =>
                    ref.read(tabletPanelOpenProvider.notifier).set(false),
              ),
            ),
          ),
        if (!open)
          Positioned(
            left: 0,
            top: 120,
            child: Material(
              key: const ValueKey('tablet-panel-toggle'),
              color: eco.sidebar,
              borderRadius:
                  const BorderRadius.horizontal(right: Radius.circular(12)),
              child: InkWell(
                borderRadius:
                    const BorderRadius.horizontal(right: Radius.circular(12)),
                onTap: () =>
                    ref.read(tabletPanelOpenProvider.notifier).set(true),
                child: Tooltip(
                  message: s.expandPanel,
                  child: const SizedBox(
                    width: 32,
                    height: 64,
                    child: Icon(Icons.chevron_right),
                  ),
                ),
              ),
            ),
          ),
      ],
    );
  }
}

class _MobileLayout extends ConsumerWidget {
  const _MobileLayout();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = context.s;
    return Stack(
      children: [
        const Positioned.fill(child: MapStack()),
        Positioned(
          left: Dimens.md,
          bottom: 36,
          child: FilledButton.tonalIcon(
            key: const ValueKey('mobile-open-panel'),
            onPressed: () => _openSheet(context),
            icon: const Icon(Icons.expand_less),
            label: Text(s.openPanel),
          ),
        ),
      ],
    );
  }

  void _openSheet(BuildContext context) {
    showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      useSafeArea: true,
      showDragHandle: true,
      builder: (ctx) => const _MobileSheet(),
    );
  }
}

class _MobileSheet extends ConsumerWidget {
  const _MobileSheet();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    // Picking an observation reveals the map + its detail card.
    ref.listen<String?>(selectedObservationIdProvider, (_, id) {
      if (id != null && Navigator.of(context).canPop()) {
        Navigator.of(context).pop();
      }
    });
    return SizedBox(
      height: MediaQuery.sizeOf(context).height * 0.8,
      child: const SidePanel(
        showHeader: false,
        visibleTabs: [
          PanelTabIds.information,
          PanelTabIds.observations,
          PanelTabIds.officialUpdates,
          PanelTabIds.simulation,
        ],
      ),
    );
  }
}

/// `/ai` route: full-screen AI tab on mobile, panel tab on wide screens.
class AiPage extends ConsumerWidget {
  const AiPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final width = MediaQuery.sizeOf(context).width;
    if (screenSizeForWidth(width) != ScreenSize.mobile) {
      // Wide layouts show the assistant as a panel tab next to the map.
      WidgetsBinding.instance.addPostFrameCallback((_) {
        ref.read(panelTabProvider.notifier).select(PanelTabIds.aiAssistant);
        ref.read(desktopPanelOpenProvider.notifier).set(true);
        ref.read(tabletPanelOpenProvider.notifier).set(true);
      });
      return const MapHomePage(initialTab: PanelTabIds.aiAssistant);
    }
    final registry = ref.watch(extensionRegistryProvider);
    return Column(
      children: [
        const Padding(
          padding: EdgeInsets.all(Dimens.md),
          child: Align(alignment: Alignment.centerLeft, child: ActiveRegionChip()),
        ),
        Expanded(
          child: buildPanelTab(context, registry, PanelTabIds.aiAssistant),
        ),
      ],
    );
  }
}

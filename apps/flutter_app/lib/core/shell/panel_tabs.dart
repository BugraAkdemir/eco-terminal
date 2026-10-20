import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../features/maps/presentation/panel/information_tab.dart';
import '../../features/maps/presentation/panel/observations_tab.dart';
import '../../shared_widgets/shared_widgets.dart';
import '../extensions/extension_registry.dart';
import '../l10n/strings.dart';

/// Static description of a panel tab; content comes from [buildPanelTab].
class PanelTabSpec {
  const PanelTabSpec(this.id, this.icon, this.label);
  final String id;
  final IconData icon;
  final String Function(AppStrings s) label;
}

final List<PanelTabSpec> panelTabSpecs = [
  PanelTabSpec(PanelTabIds.information, Icons.info_outline, (s) => s.tabInformation),
  PanelTabSpec(PanelTabIds.observations, Icons.visibility_outlined, (s) => s.tabObservations),
  PanelTabSpec(PanelTabIds.aiAssistant, Icons.auto_awesome, (s) => s.tabAi),
  PanelTabSpec(PanelTabIds.officialUpdates, Icons.newspaper, (s) => s.tabOfficial),
  PanelTabSpec(PanelTabIds.simulation, Icons.query_stats, (s) => s.tabSimulation),
];

/// Content of a tab: a registered extension builder, the built-in tab, or an
/// honest "module not configured" empty state.
Widget buildPanelTab(
  BuildContext context,
  ExtensionRegistry registry,
  String tabId,
) {
  final custom = registry.panelTab(tabId);
  if (custom != null) return KeyedSubtree(key: ValueKey('tab-$tabId'), child: custom(context));
  switch (tabId) {
    case PanelTabIds.information:
      return const InformationTab(key: ValueKey('tab-information'));
    case PanelTabIds.observations:
      return const ObservationsTab(key: ValueKey('tab-observations'));
    default:
      return ModuleNotConfigured(key: ValueKey('tab-$tabId'));
  }
}

/// Default for unregistered extension tabs. No fake data.
class ModuleNotConfigured extends StatelessWidget {
  const ModuleNotConfigured({super.key});

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

class PanelTabController extends Notifier<String> {
  @override
  String build() => PanelTabIds.information;
  void select(String id) => state = id;
}

/// Currently selected side-panel tab.
final panelTabProvider =
    NotifierProvider<PanelTabController, String>(PanelTabController.new);

/// Desktop panel width as a fraction of the available width.
class PanelFractionController extends Notifier<double> {
  static const min = 0.2;
  static const max = 0.5;
  @override
  double build() => 0.28;
  void set(double v) => state = v.clamp(min, max);
}

final panelFractionProvider =
    NotifierProvider<PanelFractionController, double>(PanelFractionController.new);

class PanelOpenController extends Notifier<bool> {
  @override
  bool build() => true;
  void set(bool v) => state = v;
}

/// Desktop: panel expanded (true) or collapsed to a rail.
final desktopPanelOpenProvider =
    NotifierProvider<PanelOpenController, bool>(PanelOpenController.new);

/// Tablet: overlay panel visible.
final tabletPanelOpenProvider =
    NotifierProvider<PanelOpenController, bool>(PanelOpenController.new);

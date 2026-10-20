import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../features/maps/presentation/map_controls.dart';
import '../extensions/extension_registry.dart';
import '../l10n/strings.dart';
import '../theme/tokens.dart';
import '../../shared_widgets/contour_texture.dart';
import 'panel_tabs.dart';

/// "Information & Intelligence" panel: title, active-region chip, 5 tabs.
/// [visibleTabs] lets the mobile sheet omit tabs that live in the bottom nav.
class SidePanel extends ConsumerWidget {
  const SidePanel({
    super.key,
    this.onCollapse,
    this.visibleTabs,
    this.showHeader = true,
  });

  final VoidCallback? onCollapse;
  final List<String>? visibleTabs;
  final bool showHeader;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final eco = context.eco;
    final s = context.s;
    final registry = ref.watch(extensionRegistryProvider);
    final specs = [
      for (final t in panelTabSpecs)
        if (visibleTabs == null || visibleTabs!.contains(t.id)) t,
    ];
    var current = ref.watch(panelTabProvider);
    if (!specs.any((t) => t.id == current)) current = specs.first.id;

    return Material(
      color: eco.sidebar,
      child: ContourBackground(
        seed: 3,
        child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          if (showHeader)
            Padding(
              padding: const EdgeInsets.fromLTRB(Dimens.lg, Dimens.md, Dimens.sm, 0),
              child: Row(
                children: [
                  Expanded(
                    child: Text(
                      s.infoPanel,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: Theme.of(context).textTheme.titleSmall?.copyWith(
                            fontWeight: FontWeight.w700,
                          ),
                    ),
                  ),
                  if (onCollapse != null)
                    IconButton(
                      tooltip: s.collapsePanel,
                      onPressed: onCollapse,
                      icon: const Icon(Icons.chevron_left),
                    ),
                ],
              ),
            ),
          Padding(
            padding: const EdgeInsets.fromLTRB(Dimens.lg, Dimens.sm, Dimens.lg, Dimens.sm),
            child: const Align(
              alignment: Alignment.centerLeft,
              child: ActiveRegionChip(),
            ),
          ),
          _PanelTabBar(
            specs: specs,
            current: current,
            onSelect: (id) => ref.read(panelTabProvider.notifier).select(id),
          ),
          const Divider(),
          Expanded(child: buildPanelTab(context, registry, current)),
        ],
      ),
      ),
    );
  }
}

class _PanelTabBar extends StatelessWidget {
  const _PanelTabBar({
    required this.specs,
    required this.current,
    required this.onSelect,
  });

  final List<PanelTabSpec> specs;
  final String current;
  final ValueChanged<String> onSelect;

  @override
  Widget build(BuildContext context) {
    final eco = context.eco;
    final s = context.s;
    return SizedBox(
      height: 48,
      child: SingleChildScrollView(
        scrollDirection: Axis.horizontal,
        padding: const EdgeInsets.symmetric(horizontal: Dimens.sm),
        child: Row(
          children: [
            for (final t in specs)
              _TabButton(
                key: ValueKey('panel-tab-${t.id}'),
                icon: t.icon,
                label: t.label(s),
                selected: t.id == current,
                color: t.id == current ? eco.teal : eco.textMuted,
                onTap: () => onSelect(t.id),
              ),
          ],
        ),
      ),
    );
  }
}

class _TabButton extends StatelessWidget {
  const _TabButton({
    super.key,
    required this.icon,
    required this.label,
    required this.selected,
    required this.color,
    required this.onTap,
  });

  final IconData icon;
  final String label;
  final bool selected;
  final Color color;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      selected: selected,
      button: true,
      label: label,
      child: InkWell(
        onTap: onTap,
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: Dimens.md),
          decoration: BoxDecoration(
            border: Border(
              bottom: BorderSide(
                color: selected ? color : Colors.transparent,
                width: 2,
              ),
            ),
          ),
          child: Row(
            children: [
              Icon(icon, size: 16, color: color),
              const SizedBox(width: 6),
              Text(
                label,
                style: TextStyle(
                  fontSize: 13,
                  color: color,
                  fontWeight: selected ? FontWeight.w700 : FontWeight.w500,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

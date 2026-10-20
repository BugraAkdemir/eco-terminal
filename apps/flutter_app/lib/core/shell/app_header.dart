import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../features/maps/domain/map_state.dart';
import '../../features/user_profile/data/notifications_providers.dart';
import '../../features/user_profile/presentation/notifications_view.dart';
import '../extensions/extension_registry.dart';
import '../l10n/strings.dart';
import '../models/models.dart';
import '../router/route_access.dart';
import '../session/session_controller.dart';
import '../storage/preferences.dart';
import '../theme/tokens.dart';
import 'search_fields.dart';

/// Top bar: logo, global search, region search, layer menu, date range,
/// notifications bell, user menu. Adapts to the available width.
class AppHeader extends ConsumerWidget implements PreferredSizeWidget {
  const AppHeader({super.key});

  @override
  Size get preferredSize => const Size.fromHeight(Dimens.headerHeight);

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final eco = context.eco;
    return Material(
      color: eco.surface,
      child: SafeArea(
        bottom: false,
        child: Container(
          height: Dimens.headerHeight,
          decoration: BoxDecoration(
            border: Border(bottom: BorderSide(color: eco.border)),
          ),
          padding: const EdgeInsets.symmetric(horizontal: Dimens.md),
          child: LayoutBuilder(
            builder: (context, c) {
              final w = c.maxWidth;
              final full = w >= 1240;
              final medium = w >= 900;
              final compact = w < Dimens.mobileMax;
              return Row(
                children: [
                  _Logo(showText: !compact),
                  const SizedBox(width: Dimens.md),
                  if (full) ...[
                    Flexible(
                      flex: 3,
                      child: ConstrainedBox(
                        constraints: const BoxConstraints(maxWidth: 280),
                        child: const GlobalSearchField(),
                      ),
                    ),
                    const SizedBox(width: Dimens.sm),
                    Flexible(
                      flex: 2,
                      child: ConstrainedBox(
                        constraints: const BoxConstraints(maxWidth: 220),
                        child: const RegionSearchField(),
                      ),
                    ),
                  ] else ...[
                    IconButton(
                      tooltip: context.s.search,
                      icon: const Icon(Icons.search),
                      onPressed: () => showSearchDialog(context, region: false),
                    ),
                    IconButton(
                      tooltip: context.s.regionSearchHint,
                      icon: const Icon(Icons.travel_explore),
                      onPressed: () => showSearchDialog(context, region: true),
                    ),
                  ],
                  const LayerMenuButton(),
                  const Spacer(),
                  if (medium) const DateRangeSegments() else const DateRangeMenu(),
                  const SizedBox(width: Dimens.xs),
                  const NotificationBell(),
                  const UserMenu(),
                ],
              );
            },
          ),
        ),
      ),
    );
  }
}

class _Logo extends StatelessWidget {
  const _Logo({required this.showText});
  final bool showText;

  @override
  Widget build(BuildContext context) {
    final eco = context.eco;
    return InkWell(
      onTap: () => context.go(AppPaths.home),
      borderRadius: BorderRadius.circular(8),
      child: Padding(
        padding: const EdgeInsets.all(4),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Container(
              width: 32,
              height: 32,
              decoration: BoxDecoration(
                gradient: LinearGradient(colors: [eco.green, eco.teal]),
                borderRadius: BorderRadius.circular(9),
              ),
              child: const Icon(Icons.eco, size: 20, color: Color(0xFF04210F)),
            ),
            if (showText) ...[
              const SizedBox(width: 8),
              Text(
                context.s.appName,
                style: Theme.of(context)
                    .textTheme
                    .titleMedium
                    ?.copyWith(fontWeight: FontWeight.w800),
              ),
            ],
          ],
        ),
      ),
    );
  }
}

// --- Layer menu --------------------------------------------------------------

class LayerMenuButton extends ConsumerWidget {
  const LayerMenuButton({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = context.s;
    final layers = ref.watch(mapLayersProvider);
    final density = ref.watch(densityModeProvider);
    final dark = ref.watch(mapDarkProvider);
    final registry = ref.watch(extensionRegistryProvider);

    String label(MapLayerId id) {
      switch (id) {
        case MapLayerId.citizen:
          return s.layerCitizen;
        case MapLayerId.wildlife:
          return s.layerWildlife;
        case MapLayerId.plant:
          return s.layerPlant;
        case MapLayerId.water:
          return s.layerWater;
        case MapLayerId.forest:
          return s.layerForest;
        case MapLayerId.issues:
          return s.layerIssues;
        case MapLayerId.official:
          return s.layerOfficial;
        case MapLayerId.boundaries:
          return s.layerBoundaries;
        case MapLayerId.analysis:
          return s.layerAnalysis;
      }
    }

    return MenuAnchor(
      menuChildren: [
        for (final id in MapLayerId.values)
          CheckboxMenuButton(
            closeOnActivate: false,
            value: layers.contains(id),
            onChanged: (id == MapLayerId.official &&
                    !registry.hasOverlayFor(id.overlayId))
                ? null
                : (v) => ref
                    .read(mapLayersProvider.notifier)
                    .toggle(id, v ?? false),
            child: Text(
              id == MapLayerId.official &&
                      !registry.hasOverlayFor(id.overlayId)
                  ? '${label(id)} (${s.layerOfficialUnavailable})'
                  : label(id),
            ),
          ),
        const Divider(height: 1),
        CheckboxMenuButton(
          closeOnActivate: false,
          value: density,
          onChanged: (v) =>
              ref.read(densityModeProvider.notifier).set(v ?? false),
          child: Text(s.densityMode),
        ),
        CheckboxMenuButton(
          closeOnActivate: false,
          value: dark,
          onChanged: (v) => ref.read(mapDarkProvider.notifier).set(v ?? false),
          child: Text(s.mapDark),
        ),
      ],
      builder: (context, controller, _) => IconButton(
        tooltip: s.layers,
        icon: const Icon(Icons.layers_outlined),
        onPressed: () =>
            controller.isOpen ? controller.close() : controller.open(),
      ),
    );
  }
}

// --- Date range ----------------------------------------------------------------

Future<void> _pickCustomRange(BuildContext context, WidgetRef ref) async {
  final now = ref.read(nowProvider);
  final cur = ref.read(resolvedRangeProvider);
  final picked = await showDateRangePicker(
    context: context,
    firstDate: DateTime(2000),
    lastDate: now,
    initialDateRange: DateTimeRange(start: cur.from, end: cur.to),
    helpText: context.s.customRange,
  );
  if (picked != null) {
    ref.read(dateRangeProvider.notifier).setCustom(
          DateTime.utc(picked.start.year, picked.start.month, picked.start.day),
          DateTime.utc(picked.end.year, picked.end.month, picked.end.day),
        );
  }
}

String _presetLabel(AppStrings s, DatePreset p) {
  switch (p) {
    case DatePreset.days30:
      return s.last30d;
    case DatePreset.year1:
      return s.last1y;
    case DatePreset.years5:
      return s.last5y;
    case DatePreset.custom:
      return s.custom;
  }
}

class DateRangeSegments extends ConsumerWidget {
  const DateRangeSegments({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = context.s;
    final filter = ref.watch(dateRangeProvider);
    return SegmentedButton<DatePreset>(
      showSelectedIcon: false,
      style: const ButtonStyle(
        visualDensity: VisualDensity.compact,
        tapTargetSize: MaterialTapTargetSize.shrinkWrap,
      ),
      segments: [
        for (final p in DatePreset.values)
          ButtonSegment(value: p, label: Text(_presetLabel(s, p))),
      ],
      selected: {filter.preset},
      onSelectionChanged: (v) {
        final p = v.first;
        if (p == DatePreset.custom) {
          _pickCustomRange(context, ref);
        } else {
          ref.read(dateRangeProvider.notifier).setPreset(p);
        }
      },
    );
  }
}

class DateRangeMenu extends ConsumerWidget {
  const DateRangeMenu({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = context.s;
    final filter = ref.watch(dateRangeProvider);
    return PopupMenuButton<DatePreset>(
      tooltip: s.dateRange,
      icon: const Icon(Icons.date_range),
      initialValue: filter.preset,
      onSelected: (p) {
        if (p == DatePreset.custom) {
          _pickCustomRange(context, ref);
        } else {
          ref.read(dateRangeProvider.notifier).setPreset(p);
        }
      },
      itemBuilder: (_) => [
        for (final p in DatePreset.values)
          CheckedPopupMenuItem(
            value: p,
            checked: p == filter.preset,
            child: Text(_presetLabel(s, p)),
          ),
      ],
    );
  }
}

// --- Notifications & user ------------------------------------------------------

class NotificationBell extends ConsumerWidget {
  const NotificationBell({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = context.s;
    final authed = ref.watch(sessionProvider.select((x) => x.isAuthenticated));
    final unread = ref.watch(unreadCountProvider);
    return IconButton(
      tooltip: s.notifications,
      onPressed: () {
        if (!authed) {
          context.go(
            Uri(
              path: AppPaths.login,
              queryParameters: {'from': AppPaths.home},
            ).toString(),
          );
          return;
        }
        showNotificationsDialog(context);
      },
      icon: Badge(
        isLabelVisible: unread > 0,
        label: Text(unread > 99 ? '99+' : '$unread'),
        child: const Icon(Icons.notifications_outlined),
      ),
    );
  }
}

class UserMenu extends ConsumerWidget {
  const UserMenu({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = context.s;
    final eco = context.eco;
    final session = ref.watch(sessionProvider);
    final locale = ref.watch(localeProvider);
    final themeMode = ref.watch(themeModeProvider);
    final user = session.user;

    if (!session.isAuthenticated || user == null) {
      return TextButton.icon(
        onPressed: () => context.go(AppPaths.login),
        icon: const Icon(Icons.login, size: 18),
        label: Text(s.login),
      );
    }

    final initial = (user.displayName.isNotEmpty ? user.displayName : user.email)
        .characters
        .first
        .toUpperCase();

    return PopupMenuButton<String>(
      tooltip: user.email,
      offset: const Offset(0, 48),
      onSelected: (v) async {
        switch (v) {
          case 'profile':
            context.go(AppPaths.profile);
          case 'expert':
            context.go(AppPaths.expert);
          case 'lang':
            await ref.read(localeProvider.notifier).set(
                  locale.languageCode == 'tr' ? const Locale('en') : const Locale('tr'),
                );
          case 'theme':
            await ref.read(themeModeProvider.notifier).set(
                  themeMode == ThemeMode.dark ? ThemeMode.light : ThemeMode.dark,
                );
          case 'logout':
            await ref.read(sessionProvider.notifier).logout();
        }
      },
      itemBuilder: (_) => [
        PopupMenuItem<String>(
          enabled: false,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                user.displayName.isNotEmpty ? user.displayName : user.email,
                style: const TextStyle(fontWeight: FontWeight.w700),
              ),
              Text(
                s.roleLabel(user.role),
                style: TextStyle(color: eco.textMuted, fontSize: 12),
              ),
            ],
          ),
        ),
        const PopupMenuDivider(),
        PopupMenuItem(value: 'profile', child: Text(s.profile)),
        if (user.role == UserRole.reviewer || user.role == UserRole.admin)
          PopupMenuItem(value: 'expert', child: Text(s.expertPanel)),
        PopupMenuItem(
          value: 'lang',
          child: Text('${s.language}: ${locale.languageCode == 'tr' ? 'English' : 'Türkçe'}'),
        ),
        PopupMenuItem(
          value: 'theme',
          child: Text(
            '${s.theme}: ${themeMode == ThemeMode.dark ? s.themeLight : s.themeDark}',
          ),
        ),
        const PopupMenuDivider(),
        PopupMenuItem(value: 'logout', child: Text(s.logout)),
      ],
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 6),
        child: CircleAvatar(
          radius: 16,
          backgroundColor: eco.teal.withValues(alpha: 0.25),
          child: Text(
            initial,
            style: TextStyle(color: eco.teal, fontWeight: FontWeight.w700),
          ),
        ),
      ),
    );
  }
}

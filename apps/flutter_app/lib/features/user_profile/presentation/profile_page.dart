import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/l10n/strings.dart';
import '../../../core/models/models.dart';
import '../../../core/router/route_access.dart';
import '../../../core/session/session_controller.dart';
import '../../../core/storage/preferences.dart';
import '../../../core/theme/tokens.dart';
import '../../../shared_widgets/shared_widgets.dart';
import '../../maps/data/map_providers.dart';
import '../../maps/presentation/observation_tile.dart';
import 'notifications_view.dart';

class ProfilePage extends ConsumerStatefulWidget {
  const ProfilePage({super.key});

  @override
  ConsumerState<ProfilePage> createState() => _ProfilePageState();
}

class _ProfilePageState extends ConsumerState<ProfilePage> {
  final _name = TextEditingController();
  bool _nameInit = false;
  bool _saving = false;

  @override
  void dispose() {
    _name.dispose();
    super.dispose();
  }

  Future<void> _saveName() async {
    setState(() => _saving = true);
    try {
      await ref
          .read(sessionProvider.notifier)
          .updateProfile(ProfileUpdate(displayName: _name.text.trim()));
      if (mounted) showSuccessSnack(context, context.s.saved);
    } catch (e) {
      if (mounted) showErrorSnack(context, e);
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  Future<void> _setLocale(Locale locale) async {
    await ref.read(localeProvider.notifier).set(locale);
    if (ref.read(sessionProvider).isAuthenticated) {
      try {
        await ref
            .read(sessionProvider.notifier)
            .updateProfile(ProfileUpdate(locale: locale.languageCode));
      } catch (e) {
        if (mounted) showErrorSnack(context, e);
      }
    }
  }

  Future<void> _setConsent(bool v) async {
    try {
      await ref
          .read(sessionProvider.notifier)
          .updateProfile(ProfileUpdate(locationConsent: v));
    } catch (e) {
      if (mounted) showErrorSnack(context, e);
    }
  }

  Future<void> _deleteAccount() async {
    final s = context.s;
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text(s.deleteAccountTitle),
        content: Text(s.deleteAccountBody),
        actions: [
          TextButton(
            key: const ValueKey('delete-cancel'),
            onPressed: () => Navigator.of(ctx).pop(false),
            child: Text(s.cancel),
          ),
          FilledButton(
            key: const ValueKey('delete-confirm'),
            style: FilledButton.styleFrom(
              backgroundColor: ctx.eco.critical,
              foregroundColor: Colors.white,
            ),
            onPressed: () => Navigator.of(ctx).pop(true),
            child: Text(s.deleteAccount),
          ),
        ],
      ),
    );
    if (ok != true || !mounted) return;
    try {
      await ref.read(sessionProvider.notifier).deleteAccount();
      if (!mounted) return;
      showSuccessSnack(context, s.accountDeleted);
      context.go(AppPaths.home);
    } catch (e) {
      if (mounted) showErrorSnack(context, e);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = context.s;
    final eco = context.eco;
    final session = ref.watch(sessionProvider);
    final user = session.user;
    final locale = ref.watch(localeProvider);

    if (user == null) {
      return EmptyState(
        icon: Icons.person_outline,
        title: s.signInToSee,
        action: FilledButton(
          onPressed: () => context.go(AppPaths.login),
          child: Text(s.login),
        ),
      );
    }
    if (!_nameInit) {
      _name.text = user.displayName;
      _nameInit = true;
    }

    final mine = ref.watch(myObservationsProvider);

    return Align(
      alignment: Alignment.topCenter,
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 760),
        child: ListView(
          padding: const EdgeInsets.all(Dimens.lg),
          children: [
            SolidPanel(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  SectionHeader(
                    title: s.profile,
                    subtitle: '${user.email} · ${s.roleLabel(user.role)}',
                  ),
                  const SizedBox(height: Dimens.sm),
                  TextField(
                    key: const ValueKey('profile-name'),
                    controller: _name,
                    decoration: InputDecoration(labelText: s.displayName),
                  ),
                  const SizedBox(height: Dimens.md),
                  Align(
                    alignment: Alignment.centerLeft,
                    child: FilledButton(
                      key: const ValueKey('profile-save'),
                      onPressed: _saving ? null : _saveName,
                      child: Text(s.saveProfile),
                    ),
                  ),
                  const Divider(height: Dimens.xxl),
                  Text(s.language, style: Theme.of(context).textTheme.labelLarge),
                  const SizedBox(height: Dimens.sm),
                  SegmentedButton<String>(
                    key: const ValueKey('profile-locale'),
                    showSelectedIcon: false,
                    segments: const [
                      ButtonSegment(value: 'tr', label: Text('Türkçe')),
                      ButtonSegment(value: 'en', label: Text('English')),
                    ],
                    selected: {locale.languageCode},
                    onSelectionChanged: (v) => _setLocale(Locale(v.first)),
                  ),
                  const Divider(height: Dimens.xxl),
                  SwitchListTile(
                    key: const ValueKey('profile-consent'),
                    contentPadding: EdgeInsets.zero,
                    title: Text(s.locationConsent),
                    subtitle: Padding(
                      padding: const EdgeInsets.only(top: 4),
                      child: Text(s.locationConsentHint),
                    ),
                    value: user.locationConsent,
                    onChanged: _setConsent,
                  ),
                ],
              ),
            ),
            const SizedBox(height: Dimens.lg),
            SolidPanel(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  SectionHeader(title: s.myReports, subtitle: s.myReportsHint),
                  mine.when(
                    loading: () => const SkeletonLoader(lines: 3),
                    error: (e, _) => ErrorState(
                      error: e,
                      compact: true,
                      onRetry: () => ref.invalidate(myObservationsProvider),
                    ),
                    data: (page) => page.items.isEmpty
                        ? EmptyState(
                            compact: true,
                            icon: Icons.edit_note,
                            title: s.noReportsYet,
                          )
                        : Column(
                            children: [
                              for (final o in page.items)
                                ObservationTile(
                                  observation: o,
                                  onTap: () =>
                                      context.push(AppPaths.observation(o.id)),
                                ),
                            ],
                          ),
                  ),
                ],
              ),
            ),
            const SizedBox(height: Dimens.lg),
            SolidPanel(
              padding: const EdgeInsets.symmetric(vertical: Dimens.lg),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  Padding(
                    padding: const EdgeInsets.symmetric(horizontal: Dimens.lg),
                    child: SectionHeader(title: s.notifications),
                  ),
                  const NotificationsView(shrinkWrap: true),
                ],
              ),
            ),
            const SizedBox(height: Dimens.lg),
            SolidPanel(
              borderColor: eco.critical.withValues(alpha: 0.5),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  SectionHeader(title: s.dangerZone),
                  OutlinedButton.icon(
                    key: const ValueKey('profile-logout'),
                    onPressed: () => ref.read(sessionProvider.notifier).logout(),
                    icon: const Icon(Icons.logout),
                    label: Text(s.logout),
                  ),
                  const SizedBox(height: Dimens.sm),
                  OutlinedButton.icon(
                    key: const ValueKey('profile-delete'),
                    style: OutlinedButton.styleFrom(
                      foregroundColor: eco.critical,
                      side: BorderSide(color: eco.critical),
                    ),
                    onPressed: _deleteAccount,
                    icon: const Icon(Icons.delete_forever_outlined),
                    label: Text(s.deleteAccount),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

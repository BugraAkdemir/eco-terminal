import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../api/providers.dart';
import '../l10n/strings.dart';

/// App locale (tr default), persisted locally. Switching it while signed in
/// is mirrored to the server profile by the profile screen.
class LocaleController extends Notifier<Locale> {
  static const _key = 'pref_locale';

  @override
  Locale build() {
    final code = ref.watch(sharedPreferencesProvider).getString(_key);
    if (code == 'en') return const Locale('en');
    return AppStrings.defaultLocale;
  }

  Future<void> set(Locale locale) async {
    state = locale;
    await ref.read(sharedPreferencesProvider).setString(_key, locale.languageCode);
  }
}

final localeProvider =
    NotifierProvider<LocaleController, Locale>(LocaleController.new);

/// Light/dark UI theme. Dark first.
class ThemeModeController extends Notifier<ThemeMode> {
  static const _key = 'pref_theme';

  @override
  ThemeMode build() {
    final v = ref.watch(sharedPreferencesProvider).getString(_key);
    return v == 'light' ? ThemeMode.light : ThemeMode.dark;
  }

  Future<void> set(ThemeMode mode) async {
    state = mode;
    await ref
        .read(sharedPreferencesProvider)
        .setString(_key, mode == ThemeMode.light ? 'light' : 'dark');
  }
}

final themeModeProvider =
    NotifierProvider<ThemeModeController, ThemeMode>(ThemeModeController.new);

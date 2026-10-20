import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'core/l10n/strings.dart';
import 'core/router/app_router.dart';
import 'core/storage/preferences.dart';
import 'core/theme/app_theme.dart';

/// Root widget: theme, locale and router.
class EcoApp extends ConsumerWidget {
  const EcoApp({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final router = ref.watch(routerProvider);
    final locale = ref.watch(localeProvider);
    final themeMode = ref.watch(themeModeProvider);
    final highContrast = ref.watch(highContrastProvider);
    return MaterialApp.router(
      title: 'Eco Terminal',
      debugShowCheckedModeBanner: false,
      routerConfig: router,
      theme: highContrast ? AppTheme.highContrast() : AppTheme.light(),
      darkTheme: AppTheme.dark(),
      themeMode: highContrast ? ThemeMode.light : themeMode,
      locale: locale,
      supportedLocales: AppStrings.supportedLocales,
      localizationsDelegates: const [
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
    );
  }
}

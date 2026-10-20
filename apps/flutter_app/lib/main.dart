import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'app.dart';
import 'core/api/providers.dart';
import 'core/l10n/format.dart';
import 'core/session/session_controller.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  await initFormatting();
  final prefs = await SharedPreferences.getInstance();

  final container = ProviderContainer(
    overrides: [sharedPreferencesProvider.overrideWithValue(prefs)],
    // Failed requests surface as error states with a retry button; no
    // silent automatic retries.
    retry: (_, _) => null,
  );
  // Session restore runs while the splash is shown (router redirect).
  container.read(sessionProvider.notifier).restore();

  runApp(UncontrolledProviderScope(container: container, child: const EcoApp()));
}

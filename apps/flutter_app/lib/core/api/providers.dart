import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../config/app_config.dart';
import '../session/session_controller.dart';
import '../storage/token_store.dart';
import 'api_client.dart';
import 'services/admin_api.dart';
import 'services/ai_api.dart';
import 'services/auth_api.dart';
import 'services/feeds_api.dart';
import 'services/media_api.dart';
import 'services/notifications_api.dart';
import 'services/observations_api.dart';
import 'services/regions_api.dart';
import 'services/review_api.dart';
import 'services/simulation_api.dart';

/// Overridden in `main()` with the real instance (and in tests with a mock).
final sharedPreferencesProvider = Provider<SharedPreferences>(
  (ref) => throw UnimplementedError('sharedPreferencesProvider not overridden'),
);

final appConfigProvider =
    Provider<AppConfig>((ref) => AppConfig.fromEnvironment);

final tokenStoreProvider = Provider<TokenStore>(
  (ref) => SecureTokenStore(ref.watch(sharedPreferencesProvider)),
);

/// True while the last request reached the server; false after a transport
/// failure. Drives the offline banner.
class NetworkStatus extends Notifier<bool> {
  @override
  bool build() => true;

  void set(bool online) {
    if (state != online) state = online;
  }
}

final networkOnlineProvider =
    NotifierProvider<NetworkStatus, bool>(NetworkStatus.new);

final apiClientProvider = Provider<ApiClient>((ref) {
  return ApiClient(
    config: ref.watch(appConfigProvider),
    tokenReader: () => ref.read(sessionProvider).token,
    onUnauthorized: () => ref.read(sessionProvider.notifier).handleUnauthorized(),
    onConnectivity: (online) =>
        ref.read(networkOnlineProvider.notifier).set(online),
  );
});

final authApiProvider = Provider((ref) => AuthApi(ref.watch(apiClientProvider)));
final systemApiProvider =
    Provider((ref) => SystemApi(ref.watch(apiClientProvider)));
final observationsApiProvider =
    Provider((ref) => ObservationsApi(ref.watch(apiClientProvider)));
final mediaApiProvider = Provider((ref) => MediaApi(ref.watch(apiClientProvider)));
final regionsApiProvider =
    Provider((ref) => RegionsApi(ref.watch(apiClientProvider)));
final reviewApiProvider = Provider((ref) => ReviewApi(ref.watch(apiClientProvider)));
final notificationsApiProvider =
    Provider((ref) => NotificationsApi(ref.watch(apiClientProvider)));
final aiApiProvider = Provider((ref) => AiApi(ref.watch(apiClientProvider)));
final feedsApiProvider = Provider((ref) => FeedsApi(ref.watch(apiClientProvider)));
final simulationApiProvider =
    Provider((ref) => SimulationApi(ref.watch(apiClientProvider)));
final adminApiProvider = Provider((ref) => AdminApi(ref.watch(apiClientProvider)));

/// Authorization header for image requests (media of non-public records).
final authHeadersProvider = Provider<Map<String, String>>((ref) {
  final token = ref.watch(sessionProvider.select((s) => s.token));
  return token == null || token.isEmpty
      ? const {}
      : {'Authorization': 'Bearer $token'};
});

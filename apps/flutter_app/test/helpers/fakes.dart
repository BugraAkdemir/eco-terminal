import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:eco_terminal/app.dart';
import 'package:eco_terminal/core/api/api_client.dart';
import 'package:eco_terminal/core/api/providers.dart';
import 'package:eco_terminal/core/extensions/extension_registry.dart';
import 'package:eco_terminal/core/session/session_controller.dart';
import 'package:eco_terminal/core/storage/token_store.dart';
import 'package:eco_terminal/features/maps/data/map_services.dart';
import 'package:flutter/material.dart';
import 'package:flutter_map/flutter_map.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_riverpod/misc.dart' show Override;
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// Handler gets the request and returns `(status, jsonBody)`.
typedef FakeHandler = (int, Object?) Function(RequestOptions options);

/// Dio adapter that answers from a route table; nothing touches the network.
class FakeAdapter implements HttpClientAdapter {
  FakeAdapter([Map<String, FakeHandler>? routes]) : routes = routes ?? {};

  /// Keys are `"METHOD /path"` (path relative to the API base URL).
  final Map<String, FakeHandler> routes;
  final List<RequestOptions> requests = [];

  /// When set, every request fails with this error instead.
  DioException? failWith;

  void on(String key, Object? body, {int status = 200}) =>
      routes[key] = (_) => (status, body);

  @override
  Future<ResponseBody> fetch(
    RequestOptions options,
    Stream<Uint8List>? requestStream,
    Future<void>? cancelFuture,
  ) async {
    requests.add(options);
    final err = failWith;
    if (err != null) {
      throw DioException(
        requestOptions: options,
        type: err.type,
        error: err.error,
      );
    }
    final handler = routes['${options.method} ${options.path}'];
    final (status, body) = handler != null
        ? handler(options)
        : (
            404,
            {
              'error': {'code': 'not_found', 'message': 'no fake route'},
            },
          );
    return ResponseBody.fromString(
      body == null ? '' : jsonEncode(body),
      status,
      headers: {
        Headers.contentTypeHeader: ['application/json'],
      },
    );
  }

  @override
  void close({bool force = false}) {}
}

class BlankTileProvider extends TileProvider {
  static final _png = base64Decode(
    'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==',
  );

  @override
  ImageProvider getImage(TileCoordinates coordinates, TileLayer options) =>
      MemoryImage(_png);
}

class FakeLocation implements LocationService {
  @override
  Future<LocationResult> current() async =>
      const LocationResult(LocationOutcome.denied);
}

Map<String, dynamic> userJson({String role = 'citizen'}) => {
      'id': 'u1',
      'email': 'ayse@example.com',
      'role': role,
      'status': 'active',
      'display_name': 'Ayşe',
      'locale': 'tr',
      'location_consent': false,
      'created_at': '2026-01-01T00:00:00Z',
    };

/// Standard public routes with empty data.
FakeAdapter emptyApi() => FakeAdapter()
  ..on('GET /regions', {'data': []})
  ..on('GET /observations', {
    'data': [],
    'meta': {'page': 1, 'page_size': 200, 'total': 0},
  })
  ..on('GET /notifications', {'data': []})
  ..on('GET /health', {
    'data': {'status': 'ok'},
  });

List<Override> baseOverrides(
  FakeAdapter adapter, {
  String? token,
  ExtensionRegistry? registry,
}) {
  return [
    tokenStoreProvider.overrideWithValue(MemoryTokenStore(token)),
    tileProviderProvider.overrideWithValue(BlankTileProvider()),
    locationServiceProvider.overrideWithValue(FakeLocation()),
    if (registry != null)
      extensionRegistryProvider.overrideWithValue(registry),
    apiClientProvider.overrideWith((ref) => makeTestClient(ref, adapter)),
  ];
}

/// Same wiring as the real [apiClientProvider], with the fake adapter.
ApiClient makeTestClient(Ref ref, FakeAdapter adapter) => ApiClient(
      config: ref.watch(appConfigProvider),
      adapter: adapter,
      tokenReader: () => ref.read(sessionProvider).token,
      onUnauthorized: () => ref.read(sessionProvider.notifier).handleUnauthorized(),
      onConnectivity: (online) =>
          ref.read(networkOnlineProvider.notifier).set(online),
    );

/// Pumps the whole app (router, shell, providers) at [size].
Future<ProviderContainer> pumpApp(
  WidgetTester tester, {
  required Size size,
  FakeAdapter? adapter,
  String? token,
  double textScale = 1.0,
  ExtensionRegistry? registry,
}) async {
  SharedPreferences.setMockInitialValues({});
  final prefs = await SharedPreferences.getInstance();
  tester.view.physicalSize = size;
  tester.view.devicePixelRatio = 1.0;
  tester.platformDispatcher.textScaleFactorTestValue = textScale;
  addTearDown(() {
    tester.view.resetPhysicalSize();
    tester.view.resetDevicePixelRatio();
    tester.platformDispatcher.clearTextScaleFactorTestValue();
  });

  final container = ProviderContainer(
    overrides: [
      sharedPreferencesProvider.overrideWithValue(prefs),
      ...baseOverrides(adapter ?? emptyApi(), token: token, registry: registry),
    ],
    retry: (_, _) => null,
  );
  addTearDown(container.dispose);
  unawaited(container.read(sessionProvider.notifier).restore());
  await tester.pumpWidget(
    UncontrolledProviderScope(container: container, child: const EcoApp()),
  );
  await tester.pump(const Duration(milliseconds: 100));
  await tester.pump(const Duration(milliseconds: 100));
  return container;
}

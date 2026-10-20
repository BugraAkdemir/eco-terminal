import 'package:dio/dio.dart';
import 'package:eco_terminal/core/api/api_client.dart';
import 'package:eco_terminal/core/api/services/auth_api.dart';
import 'package:eco_terminal/core/api/services/observations_api.dart';
import 'package:eco_terminal/core/config/app_config.dart';
import 'package:eco_terminal/core/errors/api_exception.dart';
import 'package:eco_terminal/core/models/models.dart';
import 'package:flutter_test/flutter_test.dart';

import 'helpers/fakes.dart';

void main() {
  const config = AppConfig(apiBaseUrl: 'http://localhost:8080/api/v1');

  ApiClient client(
    FakeAdapter adapter, {
    String? token,
    void Function()? onUnauthorized,
    void Function(bool)? onConnectivity,
  }) =>
      ApiClient(
        config: config,
        adapter: adapter,
        tokenReader: () => token,
        onUnauthorized: onUnauthorized,
        onConnectivity: onConnectivity,
      );

  test('parses {data, meta} envelope and sends bbox/page params', () async {
    final adapter = FakeAdapter()
      ..on('GET /observations', {
        'data': [
          {'id': 'o1', 'lat': 41.0, 'lon': 33.0, 'status': 'verified'},
        ],
        'meta': {'page': 2, 'page_size': 50, 'total': 120},
      });
    final api = ObservationsApi(client(adapter, token: 'tok'));
    final page = await api.list(
      const ObservationQuery(bbox: [33, 41, 34, 42], page: 2, pageSize: 50),
    );
    expect(page.items.single.status, ObservationStatus.verified);
    expect(page.meta.total, 120);
    final req = adapter.requests.single;
    expect(req.queryParameters['bbox'], '33.00000,41.00000,34.00000,42.00000');
    expect(req.queryParameters['page'], 2);
    expect(req.queryParameters.containsKey('category'), isFalse);
    expect(req.headers['Authorization'], 'Bearer tok');
  });

  test('error envelope becomes a typed ApiException', () async {
    final adapter = FakeAdapter()
      ..on(
        'POST /auth/login',
        {
          'error': {
            'code': 'validation_failed',
            'message': 'E-posta geçersiz',
            'details': {'field': 'email'},
          },
        },
        status: 400,
      );
    final api = AuthApi(client(adapter));
    await expectLater(
      api.login(const LoginRequest(email: 'x', password: 'y')),
      throwsA(
        isA<ApiException>()
            .having((e) => e.code, 'code', 'validation_failed')
            .having((e) => e.status, 'status', 400)
            .having((e) => e.message, 'message', 'E-posta geçersiz')
            .having((e) => e.details?['field'], 'details', 'email'),
      ),
    );
  });

  test('401 with a token triggers onUnauthorized; without a token it does not',
      () async {
    final body = {
      'error': {'code': 'unauthorized', 'message': 'no'},
    };
    var calls = 0;
    final adapter = FakeAdapter()..on('GET /auth/me', body, status: 401);

    await expectLater(
      AuthApi(client(adapter, token: 't', onUnauthorized: () => calls++)).me(),
      throwsA(isA<ApiException>().having((e) => e.isUnauthorized, 'u', true)),
    );
    expect(calls, 1);

    await expectLater(
      AuthApi(client(adapter, onUnauthorized: () => calls++)).me(),
      throwsA(isA<ApiException>()),
    );
    expect(calls, 1, reason: 'a bad login must not clear a session');
  });

  test('non-envelope error bodies still map by status code', () async {
    final adapter = FakeAdapter()..on('GET /health', null, status: 429);
    await expectLater(
      SystemApi(client(adapter)).health(),
      throwsA(
        isA<ApiException>().having((e) => e.code, 'code', ApiErrorCodes.rateLimited),
      ),
    );
  });

  test('204 No Content resolves without data', () async {
    final adapter = FakeAdapter()..on('DELETE /auth/me', null, status: 204);
    await AuthApi(client(adapter, token: 't')).deleteMe();
  });

  test('connection error -> network_unavailable and connectivity=false',
      () async {
    final adapter = FakeAdapter()
      ..failWith = DioException(
        requestOptions: RequestOptions(),
        type: DioExceptionType.connectionError,
      );
    final states = <bool>[];
    await expectLater(
      SystemApi(client(adapter, onConnectivity: states.add)).health(),
      throwsA(
        isA<ApiException>()
            .having((e) => e.code, 'code', ApiErrorCodes.networkUnavailable)
            .having((e) => e.isNetwork, 'isNetwork', true),
      ),
    );
    expect(states, [false]);
  });

  test('timeouts -> timeout code; success -> connectivity=true', () async {
    final adapter = FakeAdapter()
      ..failWith = DioException(
        requestOptions: RequestOptions(),
        type: DioExceptionType.receiveTimeout,
      );
    await expectLater(
      SystemApi(client(adapter)).health(),
      throwsA(isA<ApiException>().having((e) => e.code, 'code', ApiErrorCodes.timeout)),
    );

    final ok = FakeAdapter()
      ..on('GET /health', {
        'data': {'status': 'ok'},
      });
    final states = <bool>[];
    final h = await SystemApi(client(ok, onConnectivity: states.add)).health();
    expect(h.status, 'ok');
    expect(states, [true]);
  });

  test('AppConfig resolves relative media URLs', () {
    expect(
      config.resolveUrl('/api/v1/media/abc'),
      'http://localhost:8080/api/v1/media/abc',
    );
    expect(config.mediaUrl('abc', thumb: true),
        'http://localhost:8080/api/v1/media/abc?thumb=1');
  });
}

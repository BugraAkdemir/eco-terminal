import 'dart:convert';

import 'package:dio/dio.dart';

import '../config/app_config.dart';
import '../errors/api_exception.dart';
import '../models/json.dart';
import '../models/misc_models.dart';

/// Parsed success envelope `{data, meta}`.
class ApiEnvelope {
  const ApiEnvelope({this.data, this.meta});
  final Object? data;
  final PageMeta? meta;

  /// `data` as an object (empty map when absent).
  Json get object => asMap(data);

  /// `data` as a list of objects.
  List<Json> get list =>
      data is List ? (data as List).map(asMap).toList() : const [];
}

typedef TokenReader = String? Function();

/// Thin Dio wrapper that speaks the Eco Terminal envelope:
/// - success `{data, meta}` -> [ApiEnvelope]
/// - failure `{error:{code,message,details}}` -> [ApiException]
/// - transport failures -> [ApiException] with `network_unavailable`/`timeout`
class ApiClient {
  ApiClient({
    required this.config,
    this.tokenReader,
    this.onUnauthorized,
    this.onConnectivity,
    Dio? dio,
    HttpClientAdapter? adapter,
  }) : dio = dio ?? Dio() {
    this.dio.options = this.dio.options.copyWith(
          baseUrl: config.apiBaseUrl,
          connectTimeout: const Duration(seconds: 10),
          sendTimeout: const Duration(seconds: 60),
          receiveTimeout: const Duration(seconds: 30),
          // Every HTTP status reaches [send]; we map errors ourselves.
          validateStatus: (_) => true,
          headers: {'Accept': 'application/json'},
        );
    if (adapter != null) this.dio.httpClientAdapter = adapter;
    this.dio.interceptors.add(_AuthInterceptor(this));
  }

  final AppConfig config;
  final Dio dio;

  /// Returns the current bearer token, or null when logged out.
  final TokenReader? tokenReader;

  /// Called when an *authenticated* request is answered with 401: the session
  /// must be cleared and the user redirected to login.
  final void Function()? onUnauthorized;

  /// Called with `true` after any answered request and `false` after a
  /// transport failure. Drives the offline banner.
  final void Function(bool online)? onConnectivity;

  Future<ApiEnvelope> get(
    String path, {
    Map<String, Object?>? query,
    CancelToken? cancelToken,
  }) =>
      send('GET', path, query: query, cancelToken: cancelToken);

  Future<ApiEnvelope> post(String path, {Object? body}) =>
      send('POST', path, body: body);

  Future<ApiEnvelope> patch(String path, {Object? body}) =>
      send('PATCH', path, body: body);

  Future<ApiEnvelope> delete(String path) => send('DELETE', path);

  Future<ApiEnvelope> postForm(
    String path,
    FormData form, {
    void Function(int sent, int total)? onSendProgress,
  }) =>
      send('POST', path, body: form, onSendProgress: onSendProgress);

  Future<ApiEnvelope> send(
    String method,
    String path, {
    Map<String, Object?>? query,
    Object? body,
    CancelToken? cancelToken,
    void Function(int sent, int total)? onSendProgress,
  }) async {
    final Response<dynamic> res;
    try {
      res = await dio.request<dynamic>(
        path,
        data: body,
        queryParameters: _cleanQuery(query),
        cancelToken: cancelToken,
        onSendProgress: onSendProgress,
        options: Options(method: method),
      );
    } on DioException catch (e) {
      final ex = _mapDioException(e);
      if (ex.isNetwork) onConnectivity?.call(false);
      throw ex;
    }
    onConnectivity?.call(true);

    final status = res.statusCode ?? 0;
    final decoded = _decode(res.data);
    if (status >= 400 || status == 0) {
      throw _errorFromBody(status, decoded);
    }
    if (decoded is Map && decoded.containsKey('error') && !decoded.containsKey('data')) {
      throw _errorFromBody(status, decoded);
    }
    if (decoded == null) return const ApiEnvelope();
    if (decoded is Map) {
      final meta = decoded['meta'];
      return ApiEnvelope(
        data: decoded['data'],
        meta: meta is Map ? PageMeta.fromJson(asMap(meta)) : null,
      );
    }
    throw const ApiException(
      code: ApiErrorCodes.badResponse,
      message: 'Beklenmeyen sunucu yanıtı.',
    );
  }

  static Map<String, dynamic>? _cleanQuery(Map<String, Object?>? q) {
    if (q == null) return null;
    final out = <String, dynamic>{};
    q.forEach((k, v) {
      if (v == null) return;
      if (v is String && v.isEmpty) return;
      out[k] = v is DateTime ? v.toUtc().toIso8601String() : v;
    });
    return out.isEmpty ? null : out;
  }

  static Object? _decode(Object? raw) {
    if (raw == null) return null;
    if (raw is String) {
      if (raw.trim().isEmpty) return null;
      try {
        return jsonDecode(raw);
      } on FormatException {
        return raw;
      }
    }
    return raw;
  }

  static ApiException _errorFromBody(int status, Object? body) {
    if (body is Map && body['error'] is Map) {
      final e = asMap(body['error']);
      return ApiException(
        code: str(e['code'], _codeForStatus(status)),
        message: str(e['message'], 'HTTP $status'),
        status: status,
        details: asMapOrNull(e['details']),
      );
    }
    return ApiException(
      code: _codeForStatus(status),
      message: 'HTTP $status',
      status: status,
    );
  }

  static String _codeForStatus(int status) {
    switch (status) {
      case 400:
        return ApiErrorCodes.validationFailed;
      case 401:
        return ApiErrorCodes.unauthorized;
      case 403:
        return ApiErrorCodes.forbidden;
      case 404:
        return ApiErrorCodes.notFound;
      case 409:
        return ApiErrorCodes.conflict;
      case 413:
        return ApiErrorCodes.payloadTooLarge;
      case 415:
        return ApiErrorCodes.unsupportedMediaType;
      case 429:
        return ApiErrorCodes.rateLimited;
      case 503:
        return ApiErrorCodes.aiNotConfigured;
      default:
        return status >= 500 ? ApiErrorCodes.internal : ApiErrorCodes.unknown;
    }
  }

  static ApiException _mapDioException(DioException e) {
    switch (e.type) {
      case DioExceptionType.connectionTimeout:
      case DioExceptionType.sendTimeout:
      case DioExceptionType.receiveTimeout:
      case DioExceptionType.transformTimeout:
        return const ApiException(
          code: ApiErrorCodes.timeout,
          message: 'Sunucu zaman aşımına uğradı.',
        );
      case DioExceptionType.cancel:
        return const ApiException(
          code: ApiErrorCodes.cancelled,
          message: 'İstek iptal edildi.',
        );
      case DioExceptionType.connectionError:
      case DioExceptionType.badCertificate:
      case DioExceptionType.unknown:
      case DioExceptionType.badResponse:
        return const ApiException(
          code: ApiErrorCodes.networkUnavailable,
          message: 'Sunucuya ulaşılamıyor.',
        );
    }
  }
}

class _AuthInterceptor extends Interceptor {
  _AuthInterceptor(this.client);
  final ApiClient client;

  @override
  void onRequest(RequestOptions options, RequestInterceptorHandler handler) {
    final token = client.tokenReader?.call();
    if (token != null && token.isNotEmpty) {
      options.headers['Authorization'] = 'Bearer $token';
    }
    handler.next(options);
  }

  @override
  void onResponse(
    Response<dynamic> response,
    ResponseInterceptorHandler handler,
  ) {
    final hadToken =
        response.requestOptions.headers.containsKey('Authorization');
    if (response.statusCode == 401 && hadToken) {
      client.onUnauthorized?.call();
    }
    handler.next(response);
  }
}

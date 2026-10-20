import 'dart:convert';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../api/providers.dart';
import '../errors/api_exception.dart';
import '../models/models.dart';

enum SessionStatus { restoring, authenticated, anonymous }

class SessionState {
  const SessionState({
    this.status = SessionStatus.restoring,
    this.user,
    this.token,
    this.expired = false,
  });

  final SessionStatus status;
  final User? user;
  final String? token;

  /// True after a 401 cleared the session; the login screen shows a notice.
  final bool expired;

  bool get isAuthenticated => status == SessionStatus.authenticated;
  bool get isRestoring => status == SessionStatus.restoring;
  UserRole? get role => user?.role;

  SessionState copyWith({
    SessionStatus? status,
    User? user,
    String? token,
    bool? expired,
  }) =>
      SessionState(
        status: status ?? this.status,
        user: user ?? this.user,
        token: token ?? this.token,
        expired: expired ?? this.expired,
      );
}

const _cachedUserKey = 'cached_user';

/// Owns login state: restore on start, login/register/logout, profile
/// updates, account deletion and the 401 handler used by the API client.
class SessionController extends Notifier<SessionState> {
  @override
  SessionState build() => const SessionState();

  /// Reads the stored token and validates it with GET /auth/me.
  Future<void> restore() async {
    final store = ref.read(tokenStoreProvider);
    String? token;
    try {
      token = await store.read();
    } catch (_) {
      token = null;
    }
    if (token == null || token.isEmpty) {
      state = const SessionState(status: SessionStatus.anonymous);
      return;
    }
    state = SessionState(status: SessionStatus.restoring, token: token);
    try {
      final user = await ref.read(authApiProvider).me();
      await _cacheUser(user);
      state = SessionState(
        status: SessionStatus.authenticated,
        user: user,
        token: token,
      );
    } on ApiException catch (e) {
      if (e.isUnauthorized) {
        await _clear();
        state = const SessionState(status: SessionStatus.anonymous);
      } else {
        // Offline start: trust the cached profile until the network returns.
        final cached = _readCachedUser();
        state = cached == null
            ? SessionState(status: SessionStatus.anonymous, token: token)
            : SessionState(
                status: SessionStatus.authenticated,
                user: cached,
                token: token,
              );
      }
    }
  }

  /// Re-fetches the user (e.g. after the connection returns).
  Future<void> refreshUser() async {
    if (state.token == null) return;
    try {
      final user = await ref.read(authApiProvider).me();
      await _cacheUser(user);
      state = state.copyWith(status: SessionStatus.authenticated, user: user);
    } on ApiException {
      // handleUnauthorized covers 401; other errors keep the cached user.
    }
  }

  Future<User> login(String email, String password) async {
    final session = await ref
        .read(authApiProvider)
        .login(LoginRequest(email: email.trim(), password: password));
    await _adopt(session);
    return session.user;
  }

  Future<User> register(String email, String password,
      {String? displayName}) async {
    final session = await ref.read(authApiProvider).register(
          RegisterRequest(
            email: email.trim(),
            password: password,
            displayName:
                (displayName == null || displayName.trim().isEmpty)
                    ? null
                    : displayName.trim(),
          ),
        );
    await _adopt(session);
    return session.user;
  }

  Future<void> logout() async {
    try {
      await ref.read(authApiProvider).logout();
    } on ApiException {
      // Even if the server is unreachable we drop the local session.
    }
    await _clear();
    state = const SessionState(status: SessionStatus.anonymous);
  }

  Future<void> updateProfile(ProfileUpdate update) async {
    final user = await ref.read(authApiProvider).updateMe(update);
    await _cacheUser(user);
    state = state.copyWith(user: user);
  }

  /// DELETE /auth/me, then clears everything local.
  Future<void> deleteAccount() async {
    await ref.read(authApiProvider).deleteMe();
    await _clear();
    state = const SessionState(status: SessionStatus.anonymous);
  }

  /// Called by the API client on a 401 for an authenticated request.
  void handleUnauthorized() {
    if (state.status == SessionStatus.anonymous && state.token == null) return;
    _clear();
    state = const SessionState(status: SessionStatus.anonymous, expired: true);
  }

  void acknowledgeExpired() {
    if (state.expired) state = state.copyWith(expired: false);
  }

  Future<void> _adopt(AuthSession session) async {
    await ref.read(tokenStoreProvider).write(session.token);
    await _cacheUser(session.user);
    state = SessionState(
      status: SessionStatus.authenticated,
      user: session.user,
      token: session.token,
    );
  }

  Future<void> _clear() async {
    try {
      await ref.read(tokenStoreProvider).clear();
      await ref.read(sharedPreferencesProvider).remove(_cachedUserKey);
    } catch (_) {}
  }

  Future<void> _cacheUser(User user) async {
    try {
      await ref
          .read(sharedPreferencesProvider)
          .setString(_cachedUserKey, jsonEncode(user.toJson()));
    } catch (_) {}
  }

  User? _readCachedUser() {
    try {
      final raw = ref.read(sharedPreferencesProvider).getString(_cachedUserKey);
      if (raw == null) return null;
      return User.fromJson(asMapFromJson(raw));
    } catch (_) {
      return null;
    }
  }
}

Map<String, dynamic> asMapFromJson(String raw) =>
    Map<String, dynamic>.from(jsonDecode(raw) as Map);

final sessionProvider =
    NotifierProvider<SessionController, SessionState>(SessionController.new);

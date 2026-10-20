import '../../models/models.dart';
import '../api_client.dart';

/// `/auth/*` and `/health`.
class AuthApi {
  const AuthApi(this._c);
  final ApiClient _c;

  Future<AuthSession> register(RegisterRequest req) async =>
      AuthSession.fromJson((await _c.post('/auth/register', body: req.toJson())).object);

  Future<AuthSession> login(LoginRequest req) async =>
      AuthSession.fromJson((await _c.post('/auth/login', body: req.toJson())).object);

  Future<void> logout() async {
    await _c.post('/auth/logout');
  }

  Future<User> me() async => User.fromJson((await _c.get('/auth/me')).object);

  Future<User> updateMe(ProfileUpdate update) async =>
      User.fromJson((await _c.patch('/auth/me', body: update.toJson())).object);

  /// Anonymizes the profile and removes personal data (204).
  Future<void> deleteMe() async {
    await _c.delete('/auth/me');
  }
}

class SystemApi {
  const SystemApi(this._c);
  final ApiClient _c;

  Future<HealthStatus> health() async =>
      HealthStatus.fromJson((await _c.get('/health')).object);
}

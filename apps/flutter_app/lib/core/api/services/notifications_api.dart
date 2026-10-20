import '../../models/models.dart';
import '../api_client.dart';

/// `/notifications*`.
class NotificationsApi {
  const NotificationsApi(this._c);
  final ApiClient _c;

  Future<List<AppNotification>> list({bool? unread}) async {
    final env = await _c.get('/notifications', query: {'unread': unread});
    return env.list.map(AppNotification.fromJson).toList();
  }

  Future<void> markRead(String id) async {
    await _c.post('/notifications/$id/read');
  }
}

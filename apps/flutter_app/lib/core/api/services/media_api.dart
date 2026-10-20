import 'dart:typed_data';

import 'package:dio/dio.dart';

import '../../config/app_config.dart';
import '../../models/models.dart';
import '../api_client.dart';

/// `/observations/{id}/media` (upload) and `/media/{id}` (URLs).
class MediaApi {
  const MediaApi(this._c);
  final ApiClient _c;

  AppConfig get _config => _c.config;

  /// Uploads one image (multipart field `file`; jpeg/png/webp).
  Future<Media> upload(
    String observationId, {
    required Uint8List bytes,
    required String filename,
    String mimeType = 'image/jpeg',
    void Function(int sent, int total)? onProgress,
  }) async {
    final form = FormData.fromMap({
      'file': MultipartFile.fromBytes(
        bytes,
        filename: filename,
        contentType: DioMediaType.parse(mimeType),
      ),
    });
    final env = await _c.postForm(
      '/observations/$observationId/media',
      form,
      onSendProgress: onProgress,
    );
    return Media.fromJson(env.object);
  }

  /// Absolute URL for displaying a [Media] (full size).
  String urlFor(Media m) =>
      m.url.isNotEmpty ? _config.resolveUrl(m.url) : _config.mediaUrl(m.id);

  /// Absolute URL of the 400px thumbnail.
  String thumbUrlFor(Media m) => m.thumbUrl.isNotEmpty
      ? _config.resolveUrl(m.thumbUrl)
      : _config.mediaUrl(m.id, thumb: true);

  /// Thumbnail URL for an observation list item.
  String thumbUrlForId(String mediaId) =>
      _config.mediaUrl(mediaId, thumb: true);
}

package io.codeallremote.car.android.notifications

import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Build
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import io.codeallremote.car.android.MainActivity
import io.codeallremote.car.android.ui.navigation.Routes

/**
 * Builds and posts notifications (M2-06/19 notification side).
 *
 * Rules (docs/19, docs/15 §Notification privacy):
 * - Payload contains identifiers only; the notification text is generic.
 * - Opening the notification deep-links into CAR, which reauthenticates and
 *   fetches current state (a stale/resolved approval shows its final state,
 *   not an actionable button).
 * - Lock-screen visibility respects the user's privacy preference.
 */
class NotificationRouter(private val context: Context) {

    init { ensureChannel() }

    fun postApproval(payload: PushPayload, lockScreenPrivate: Boolean) {
        val deepLink = Routes.approvalDeepLink(payload.serverId, payload.resourceId)
        post(payload, deepLink, lockScreenPrivate)
    }

    fun postSessionEvent(payload: PushPayload, lockScreenPrivate: Boolean) {
        val deepLink = Routes.sessionDeepLink(payload.serverId, payload.resourceId)
        post(payload, deepLink, lockScreenPrivate)
    }

    private fun post(payload: PushPayload, deepLink: String, lockScreenPrivate: Boolean) {
        val intent = Intent(context, MainActivity::class.java).apply {
            action = Intent.ACTION_VIEW
            data = Uri.parse(deepLink)
            flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP
        }
        val pi = PendingIntent.getActivity(
            context,
            payload.resourceId.hashCode(),
            intent,
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )

        val builder = NotificationCompat.Builder(context, CHANNEL_ID)
            .setSmallIcon(android.R.drawable.stat_sys_download_done)
            .setContentTitle(payload.genericTitle())
            .setContentText(payload.genericBody())
            .setContentIntent(pi)
            .setAutoCancel(true)
            // Identifiers only; no expanded content with secrets.
            .setVisibility(
                if (lockScreenPrivate) NotificationCompat.VISIBILITY_PRIVATE
                else NotificationCompat.VISIBILITY_PUBLIC
            )

        try {
            NotificationManagerCompat.from(context).notify(payload.resourceId.hashCode(), builder.build())
        } catch (se: SecurityException) {
            // POST_NOTIFICATIONS not granted; the deep-link still works on app open.
        }
    }

    private fun ensureChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channel = NotificationChannel(
                CHANNEL_ID,
                "CAR alerts",
                NotificationManager.IMPORTANCE_HIGH,
            ).apply {
                description = "Approval requests and run outcomes"
                // No sensitive content in the channel description.
            }
            val mgr = context.getSystemService(NotificationManager::class.java)
            mgr.createNotificationChannel(channel)
        }
    }

    companion object {
        private const val CHANNEL_ID = "car_alerts"
    }
}

package io.codeallremote.car.android.notifications

import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.Service
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.IBinder
import androidx.core.app.NotificationCompat
import io.codeallremote.car.android.data.CarClient
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch

class CarConnectionService : Service() {

    companion object {
        const val EXTRA_SERVER_ID = "server_id"
        private const val CHANNEL_ID = "car_connection"
        private const val NOTIFICATION_ID = 1001
    }

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private var client: CarClient? = null
    private lateinit var router: NotificationRouter

    @Volatile
    private var channelCreated = false

    override fun onCreate() {
        super.onCreate()
        router = NotificationRouter(applicationContext)
        ensureChannel()
    }

    private fun ensureChannel() {
        if (channelCreated) return
        val manager = getSystemService(NotificationManager::class.java) ?: return
        val channel = NotificationChannel(
            CHANNEL_ID,
            "CAR Connection",
            NotificationManager.IMPORTANCE_LOW
        )
        manager.createNotificationChannel(channel)
        channelCreated = true
    }

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val serverId = intent?.getStringExtra(EXTRA_SERVER_ID) ?: run {
            stopSelf()
            return START_NOT_STICKY
        }

        ensureChannel()

        val notification = NotificationCompat.Builder(this, CHANNEL_ID)
            .setSmallIcon(android.R.drawable.stat_notify_sync)
            .setContentTitle("CAR connected")
            .setOngoing(true)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .build()

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            startForeground(
                NOTIFICATION_ID,
                notification,
                ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC
            )
        } else {
            startForeground(NOTIFICATION_ID, notification)
        }

        val bound = io.codeallremote.car.android.data.ActiveServer.bind(applicationContext)
        val c = io.codeallremote.car.android.data.ActiveServer.client()
        if (!bound || c == null) {
            stopSelf()
            return START_NOT_STICKY
        }
        client = c

        scope.launch {
            c.ws.events.collect { event ->
                val m = NotificationMapper.map(serverId, event) ?: return@collect
                if (m.isApproval) {
                    router.postApproval(m.payload, lockScreenPrivate = true)
                } else {
                    router.postSessionEvent(m.payload, lockScreenPrivate = true)
                }
            }
        }

        // ActiveServer.bind() already started the shared WS; do NOT start another.

        return START_STICKY
    }

    override fun onDestroy() {
        // The shared WebSocket is owned by ActiveServer and used by foreground
        // screens; do not stop it here. Only cancel this service's scope.
        scope.cancel()
        super.onDestroy()
    }
}

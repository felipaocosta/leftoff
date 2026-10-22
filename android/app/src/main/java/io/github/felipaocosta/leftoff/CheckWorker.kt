package io.github.felipaocosta.leftoff

import android.Manifest
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import androidx.core.content.ContextCompat
import androidx.work.Constraints
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.NetworkType
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.Worker
import androidx.work.WorkerParameters
import java.io.File
import java.util.concurrent.TimeUnit
import org.json.JSONArray
import io.github.felipaocosta.leftoff.mobile.Mobile

/**
 * Checks for new chapters while the app is closed (RF-22): every 6 hours,
 * with a network, the Go core checks the due series, and the ones that got
 * new chapters become one Android notification that opens New chapters.
 * The core's own notifications (ntfy, Discord, Telegram) are sent as well.
 */
class CheckWorker(context: Context, params: WorkerParameters) : Worker(context, params) {
    override fun doWork(): Result {
        val dataDir = File(applicationContext.filesDir, "leftoff").absolutePath
        val found = try {
            // Renders JavaScript pages in the background too; checks that
            // need a person fail (blocked) until the app is opened.
            Mobile.setBrowser(WebViewBrowser.get(applicationContext))
            JSONArray(Mobile.checkNow(dataDir))
        } catch (e: Exception) {
            return Result.retry()
        }
        if (found.length() > 0) notify(applicationContext, found)
        return Result.success()
    }

    companion object {
        private const val WORK = "check-new-chapters"
        private const val CHANNEL = "new_chapters"
        private const val NOTIFICATION = 1

        /** Schedules the periodic check (kept if already scheduled). */
        fun schedule(context: Context) {
            val request = PeriodicWorkRequestBuilder<CheckWorker>(6, TimeUnit.HOURS)
                .setConstraints(Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build())
                .build()
            WorkManager.getInstance(context)
                .enqueueUniquePeriodicWork(WORK, ExistingPeriodicWorkPolicy.KEEP, request)
        }

        /** One notification for the whole check: a line per series. */
        private fun notify(context: Context, found: JSONArray) {
            if (Build.VERSION.SDK_INT >= 33 &&
                ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) !=
                PackageManager.PERMISSION_GRANTED
            ) return
            val manager = context.getSystemService(NotificationManager::class.java)
            if (Build.VERSION.SDK_INT >= 26) {
                manager.createNotificationChannel(
                    NotificationChannel(CHANNEL, context.getString(R.string.channel_new_chapters), NotificationManager.IMPORTANCE_DEFAULT),
                )
            }
            val lines = (0 until found.length()).map { line(context, found.getJSONObject(it)) }
            val open = PendingIntent.getActivity(
                context, 0,
                Intent(Intent.ACTION_VIEW, Uri.parse("leftoff://news"), context, MainActivity::class.java),
                PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
            )
            val title = context.resources.getQuantityString(R.plurals.new_chapters_title, lines.size, lines.size)
            val style = NotificationCompat.InboxStyle().setBigContentTitle(title)
            lines.take(6).forEach { style.addLine(it) }
            if (lines.size > 6) style.setSummaryText(context.resources.getQuantityString(R.plurals.more_series, lines.size - 6, lines.size - 6))
            val notification = NotificationCompat.Builder(context, CHANNEL)
                .setSmallIcon(R.drawable.ic_notification)
                .setContentTitle(title)
                .setContentText(lines.joinToString(" · "))
                .setStyle(style)
                .setContentIntent(open)
                .setAutoCancel(true)
                .build()
            NotificationManagerCompat.from(context).notify(NOTIFICATION, notification)
        }

        /** "Eleceed: ch. 424" (or just the title without a number). */
        private fun line(context: Context, s: org.json.JSONObject): String {
            val title = s.optString("title")
            if (s.isNull("latest_chapter")) return title
            val n = s.getDouble("latest_chapter")
            val number = if (n == Math.floor(n)) n.toLong().toString() else n.toString()
            return context.getString(R.string.series_chapter, title, number)
        }
    }
}

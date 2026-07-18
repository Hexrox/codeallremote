# CAR Android ProGuard rules.

# Keep @Serializable data classes (kotlinx.serialization generates reflectors).
-keepattributes *Annotation*, InnerClasses
-dontnote kotlinx.serialization.**
-keepclassmembers class io.codeallremote.car.android.** {
    *** Companion;
}
-keepclasseswithmembers class io.codeallremote.car.android.** {
    kotlinx.serialization.KSerializer serializer(...);
}

# OkHttp (ships consumer rules; this is defensive).
-dontwarn okhttp3.internal.platform.**

# Keep models used over the WS/REST boundary.
-keep class io.codeallremote.car.android.net.dto.** { *; }

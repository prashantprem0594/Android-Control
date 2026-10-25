package com.appcontrol.parent

import android.app.Activity
import android.os.Bundle
import android.view.Gravity
import android.widget.LinearLayout
import android.widget.TextView

class MainActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        val content = LinearLayout(this).apply {
            gravity = Gravity.CENTER
            orientation = LinearLayout.VERTICAL
            setPadding(48, 48, 48, 48)
            addView(TextView(context).apply {
                text = "App Control Parent\n\nSecure enrollment and controls will be added after device feasibility passes."
                textSize = 20f
                gravity = Gravity.CENTER
            })
        }

        setContentView(content)
    }
}

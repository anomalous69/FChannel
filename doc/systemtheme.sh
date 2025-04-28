#!/bin/sh
#Basic script to merge default and tomorrow theme into system theme

# Light theme
printf "@media (prefers-color-scheme: light) {\n" > static/css/themes/system.css
cat static/css/themes/default.css >> static/css/themes/system.css
printf "\n}" >> static/css/themes/system.css

# Dark theme
printf "\n\n@media (prefers-color-scheme: dark) {\n" >> static/css/themes/system.css
cat static/css/themes/tomorrow.css >> static/css/themes/system.css
printf "\n}" >> static/css/themes/system.css
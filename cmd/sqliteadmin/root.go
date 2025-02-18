package main

import (
	"log"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "sqliteadmin",
	Short: "A web-based SQLite database management tool",
	Long:  `sqliteadmin is a web-based SQLite database management tool that allows you to view and manage your SQLite database through a web interface (https://sqliteadmin.dev).`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatalln(err)
	}
}

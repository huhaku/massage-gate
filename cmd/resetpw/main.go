// cmd/resetpw 重置管理员密码
// 用法: ./resetpw -data ./data -password newpassword
package main

import (
	"flag"
	"fmt"
	"log"
	"path/filepath"

	"golang.org/x/crypto/bcrypt"

	"massagegate/internal/store"
)

func main() {
	dataDir := flag.String("data", "./data", "数据目录")
	password := flag.String("password", "", "新密码 (至少6位)")
	flag.Parse()

	if *password == "" {
		fmt.Println("用法: resetpw -data ./data -password <新密码>")
		fmt.Println("  -data      数据目录 (默认: ./data)")
		fmt.Println("  -password  新密码 (至少6位)")
		fmt.Println()
		fmt.Println("示例:")
		fmt.Println("  ./resetpw -password admin123")
		fmt.Println("  ./resetpw -data /path/to/data -password mynewpass")
		return
	}

	if len(*password) < 6 {
		log.Fatal("密码长度至少6位")
	}

	db, err := store.Open(filepath.Join(*dataDir, "massage-gate.db"))
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}

	// 检查是否已初始化
	account := store.GetSetting(db, "account")
	if account == "" {
		log.Fatal("系统尚未初始化，请先访问 Web 界面完成初始化")
	}

	// 生成密码哈希
	hash, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("生成密码哈希失败: %v", err)
	}

	// 更新密码
	if err := store.SetSetting(db, "password_hash", string(hash)); err != nil {
		log.Fatalf("更新密码失败: %v", err)
	}

	fmt.Printf("✓ 密码已重置\n")
	fmt.Printf("  用户名: %s\n", account)
	fmt.Printf("  新密码: %s\n", *password)
	fmt.Printf("  请使用新密码登录: http://localhost:8080\n")
}
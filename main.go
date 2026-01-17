package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	// Создаём директорию для загрузок, если её нет
	uploadDir := "./uploads"
	if err := os.MkdirAll(uploadDir, os.ModePerm); err != nil {
		log.Fatal(err)
	}

	r := gin.Default()
	r.LoadHTMLGlob("templates/*")
	r.Static("/asserts", "./asserts")

	// Middleware для CORS (если фронт на другом домене)
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if c.Request.Method == "OPTIONS" {
			c.JSON(200, gin.H{"message": "Проверка на опции"})
			return
		}

		c.Next()
	})

	// Страница с формой для тестирования
	r.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", gin.H{})
	})

	// 📤 Загрузка одного файла
	r.POST("/upload", func(c *gin.Context) {
		// Ограничение размера файла (10 MB)
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 10<<20)

		file, err := c.FormFile("file")
		if err != nil {
			if err == http.ErrMissingFile {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Файл не предоставлен"})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": "Файл слишком большой (макс. 10MB)"})
			return
		}

		// Валидация размера файла
		if file.Size > 10<<20 { // 10 MB
			c.JSON(http.StatusBadRequest, gin.H{"error": "Файл превышает 10MB"})
			return
		}

		// Генерация уникального имени файла
		filename := fmt.Sprintf("%d_%s", time.Now().UnixNano(), file.Filename)
		filepath_to_file := filepath.Join(uploadDir, filename)

		// Сохранение файла
		if err := c.SaveUploadedFile(file, filepath_to_file); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Не удалось сохранить файл"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":  "Файл успешно загружен",
			"filename": filename,
			"size":     file.Size,
			"path":     filepath_to_file,
		})
	})

	// 📤 Загрузка нескольких файлов
	r.POST("/upload/multiple", func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 50<<20) // 50 MB для нескольких файлов

		form, err := c.MultipartForm()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		files := form.File["files"]
		if len(files) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Файлы не предоставлены"})
			return
		}

		var uploadedFiles []gin.H
		for _, file := range files {
			// Проверка размера каждого файла
			if file.Size > 10<<20 {
				continue // Пропускаем слишком большие файлы
			}

			filename := fmt.Sprintf("%d_%s", time.Now().UnixNano(), file.Filename)
			filepathToFile := filepath.Join(uploadDir, filename)

			if err := c.SaveUploadedFile(file, filepathToFile); err != nil {
				continue
			}

			uploadedFiles = append(uploadedFiles, gin.H{
				"filename": filename,
				"original": file.Filename,
				"size":     file.Size,
				"path":     filepathToFile,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"message": fmt.Sprintf("Загружено %d файлов", len(uploadedFiles)),
			"files":   uploadedFiles,
		})
	})

	// 📥 Скачивание файла
	r.GET("/download/:filename", func(c *gin.Context) {
		filename := c.Param("filename")
		filepath := filepath.Join(uploadDir, filename)

		// Проверяем существование файла
		if _, err := os.Stat(filepath); os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Файл не найден"})
			return
		}

		// Устанавливаем заголовки для скачивания
		c.Header("Content-Description", "File Transfer")
		c.Header("Content-Transfer-Encoding", "binary")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
		c.Header("Content-Type", "application/octet-stream")
		c.File(filepath)
	})

	// 📥 Стриминг файла (без скачивания)
	r.GET("/files/:filename", func(c *gin.Context) {
		filename := c.Param("filename")
		filepath1 := filepath.Join(uploadDir, filename)

		if _, err := os.Stat(filepath1); os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Файл не найден"})
			return
		}

		// Определяем Content-Type на основе расширения
		ext := filepath.Ext(filename)
		contentType := mimeTypes[ext]
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		c.Header("Content-Type", contentType)
		c.File(filepath1)
	})

	// 📋 Получение списка файлов
	r.GET("/files", func(c *gin.Context) {
		files, err := os.ReadDir(uploadDir)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Не удалось прочитать директорию"})
			return
		}

		var fileList []gin.H
		for _, file := range files {
			info, err := file.Info()
			if err != nil {
				continue
			}

			fileList = append(fileList, gin.H{
				"name":    file.Name(),
				"size":    info.Size(),
				"modTime": info.ModTime().Format(time.RFC3339),
				"isDir":   file.IsDir(),
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"count": len(fileList),
			"files": fileList,
		})
	})

	// 🗑️ Удаление файла
	r.DELETE("/files/:filename", func(c *gin.Context) {
		filename := c.Param("filename")
		filepath := filepath.Join(uploadDir, filename)

		if _, err := os.Stat(filepath); os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Файл не найден"})
			return
		}

		if err := os.Remove(filepath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Не удалось удалить файл"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Файл успешно удален"})
	})

	// 📊 Статус сервера
	r.GET("/status", func(c *gin.Context) {
		var totalSize int64
		files, _ := os.ReadDir(uploadDir)

		for _, file := range files {
			info, _ := file.Info()
			totalSize += info.Size()
		}

		c.JSON(http.StatusOK, gin.H{
			"filesCount":  len(files),
			"totalSize":   totalSize,
			"totalSizeMB": totalSize / (1 << 20),
			"uploadDir":   uploadDir,
			"serverTime":  time.Now().Format(time.RFC3339),
		})
	})

	// Запуск сервера
	fmt.Println("Сервер запущен на http://localhost:8080")
	fmt.Println("Директория для загрузок:", uploadDir)
	r.Run(":9080")
}

// MIME типы для расширений файлов
var mimeTypes = map[string]string{
	".txt":  "text/plain",
	".html": "text/html",
	".css":  "text/css",
	".js":   "application/javascript",
	".json": "application/json",
	".xml":  "application/xml",
	".pdf":  "application/pdf",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".gif":  "image/gif",
	".svg":  "image/svg+xml",
	".mp4":  "video/mp4",
	".mp3":  "audio/mpeg",
	".zip":  "application/zip",
	".rar":  "application/x-rar-compressed",
}

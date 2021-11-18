deploy:
	git push heroku main

logs:
	heroku logs -a ref-collector-2021 --tail